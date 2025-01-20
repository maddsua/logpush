package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/maddsua/logpush/service/dbops"
)

type LokiConnection struct {
	url string
}

func ParseLokiUrl(params string) (*LokiConnection, error) {

	if params = strings.TrimSpace(params); params == "" {
		return nil, nil
	}

	parsed, err := url.Parse(params)
	if err != nil {
		return nil, err
	}

	if parsed.Scheme == "" {
		return nil, fmt.Errorf("url scheme must be http or https")
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("url host is not defined")
	}

	lokiUrl := &url.URL{
		Scheme: parsed.Scheme,
		Host:   parsed.Host,
	}

	return &LokiConnection{
		url: lokiUrl.String(),
	}, nil
}

type Level string

func (lvl Level) String() string {
	switch lvl {
	case "log", "warn", "error", "debug", "info":
		return string(lvl)
	default:
		return "error"
	}
}

type UnixMilli int64

func (um UnixMilli) String(sequence int) string {

	var ts time.Time
	if um > 0 {
		ts = time.UnixMilli(int64(um))
	} else {
		ts = time.Now()
	}

	return strconv.FormatInt(ts.UnixNano()+int64(sequence), 10)
}

func (um UnixMilli) Time(sequence int) time.Time {

	var ts time.Time
	if um > 0 {
		ts = time.UnixMilli(int64(um))
	} else {
		ts = time.Now()
	}

	//	todo: ensure correct result
	return ts.Add(time.Duration(sequence))
}

type LokiHttpBatch struct {
	Streams []LokiStream `json:"streams"`
}

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]any           `json:"values"`
}

func (this *LokiConnection) PushStreams(streams []LokiStream) error {

	payload, err := lokiSerializeStreams(streams)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", this.url+"/loki/api/v1/push", payload)
	if err != nil {
		return err
	}

	req.Header.Set("content-type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent:
		return nil
	}

	contentType := resp.Header.Get("content-type")
	switch contentType {
	case "application/json", "text/plain":
		break
	default:
		return fmt.Errorf("failed to push log streams: http error (code %d, content-type: %s)", resp.StatusCode, contentType)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read error response body: %s", err.Error())
	}

	return fmt.Errorf("failed to push log streams: %s", string(responseBody))
}

func (this *LokiConnection) IngestWeb(source IngestSource, payload WebStream) {

	stream := payload.ToLokiStream(source.Stream.Name)
	if len(stream.Values) == 0 {
		slog.Warn("LOKI FORWARDER: Empty log batch",
			slog.String("stream_id", source.Stream.ID.String()),
			slog.String("remote_addr", source.RemoteAddr))
		return
	}

	if len(source.Stream.Labels) > 0 {
		var streamLabels map[string]string
		if err := json.Unmarshal(source.Stream.Labels, &streamLabels); err == nil {
			for key, val := range streamLabels {
				if mval, has := stream.Stream[key]; has {
					stream.Stream["_opt_"+key] = mval
				}
				stream.Stream[key] = val
			}
		}
	}

	for i := 0; i < webStreamPushRetryAttempts; i++ {

		if err := this.PushStreams([]LokiStream{stream}); err != nil {
			slog.Error("LOKI FORWARDER: failed to push entries",
				slog.String("err", err.Error()),
				slog.Int("attempt", i+1),
				slog.Int("of", webStreamPushRetryAttempts),
				slog.String("stream_id", source.Stream.ID.String()),
				slog.String("remote_addr", source.RemoteAddr))
			continue
		}

		break
	}

	slog.Info("LOKI FORWARDER: Wrote entries",
		slog.Int("count", len(stream.Values)),
		slog.String("stream_id", source.Stream.ID.String()),
		slog.String("remote_addr", source.RemoteAddr))
}

func lokiSerializeStreams(streams []LokiStream) (*bytes.Buffer, error) {

	payload := LokiHttpBatch{Streams: streams}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(data), nil
}

type Timescale struct {
	DB *sql.DB
}

func (this *Timescale) IngestWeb(source IngestSource, payload WebStream) {

	rows := payload.ToTimescaleRows(source.Stream.ID)
	if len(rows) == 0 {
		slog.Warn("LOKI FORWARDER: Empty log batch",
			slog.String("stream_id", source.Stream.ID.String()),
			slog.String("remote_addr", source.RemoteAddr))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tx, err := this.DB.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("TIMESCALE FORWARDER: Failed to begin DB TX",
			slog.String("err", err.Error()),
			slog.String("stream_id", source.Stream.ID.String()),
			slog.String("remote_addr", source.RemoteAddr))
		return
	}

	txq := dbops.New(tx)

	for _, row := range rows {
		if err := txq.InsertStreamEntry(ctx, row); err != nil {
			slog.Error("TIMESCALE FORWARDER: Failed to insert row",
				slog.String("err", err.Error()),
				slog.String("stream_id", source.Stream.ID.String()),
				slog.String("remote_addr", source.RemoteAddr))
		}
	}

	defer tx.Rollback()

	if err := tx.Commit(); err != nil {
		slog.Error("TIMESCALE FORWARDER: Failed to commit DB TX",
			slog.String("err", err.Error()),
			slog.String("stream_id", source.Stream.ID.String()),
			slog.String("remote_addr", source.RemoteAddr))
		return
	}

	slog.Info("TIMESCALE FORWARDER: Wrote entries",
		slog.Int("count", len(rows)),
		slog.String("stream_id", source.Stream.ID.String()),
		slog.String("remote_addr", source.RemoteAddr))
}
