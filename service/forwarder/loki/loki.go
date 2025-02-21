package loki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/ingester/streams"
)

type Loki struct {
	LokiOptions
	url string
}

type LokiOptions struct {
	Retries       int
	UseStructMeta bool
	StrictLabels  bool
}

func ParseLokiUrl(params string, opts LokiOptions) (*Loki, error) {

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

	return &Loki{
		LokiOptions: opts,
		url:         lokiUrl.String(),
	}, nil
}

func (this *Loki) Ready() error {

	req, err := http.NewRequest("GET", this.url+"/ready", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent:
		return nil
	}

	return fmt.Errorf("http status code %d", resp.StatusCode)
}

type LokiHttpBatch struct {
	Streams []LokiStream `json:"streams"`
}

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]any           `json:"values"`
}

func (this *Loki) retryNumber() int {
	if this.Retries > 0 {
		return this.Retries
	}
	return 3
}

func (this *Loki) PushStreams(streams []LokiStream) error {

	payload, err := this.serializeStreams(streams)
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

func (this *Loki) IngestWeb(streamSource *dbops.Stream, txID uuid.UUID, remoteAddr string, payload *streams.WebStream) {

	var streams []LokiStream
	if this.UseStructMeta {
		if next := this.webStreamToStructured(payload, streamSource, txID); len(next.Values) > 0 {
			streams = []LokiStream{next}
		}
	} else {
		streams = this.webStreamToLabeled(payload, streamSource, txID)
	}

	if len(streams) == 0 {
		slog.Warn("LOKI FORWARDER: Empty log batch",
			slog.String("stream_id", streamSource.ID.String()),
			slog.String("remote_addr", remoteAddr))
		return
	}

	for i := 0; i < this.retryNumber(); i++ {

		err := this.PushStreams(streams)
		if err == nil {
			break
		}

		slog.Error("LOKI FORWARDER: failed to push entries",
			slog.String("err", err.Error()),
			slog.Int("attempt", i+1),
			slog.Int("of", this.retryNumber()),
			slog.String("stream_id", streamSource.ID.String()),
			slog.String("remote_addr", remoteAddr))
	}

	slog.Debug("LOKI FORWARDER: Wrote streams",
		slog.Int("count", len(streams)),
		slog.String("stream_id", streamSource.ID.String()),
		slog.String("remote_addr", remoteAddr))
}

func (this *Loki) serializeStreams(streams []LokiStream) (*bytes.Buffer, error) {

	payload := LokiHttpBatch{Streams: streams}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(data), nil
}
