package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/guregu/null"
	"github.com/maddsua/logpush/service/logs"
)

type LogIngester struct {
	Storage logs.Collector
	Cfg     IngesterConfig
	Streams map[string]StreamConfig
}

func (this *LogIngester) ServeHTTP(wrt http.ResponseWriter, req *http.Request) {

	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		req.RemoteAddr = xff
	} else if host, _, _ := net.SplitHostPort(req.RemoteAddr); host != "" {
		req.RemoteAddr = host
	}

	if err := this.handleProcedure(req); err != nil {
		wrt.Header().Set("content-type", "text/plain")
		wrt.WriteHeader(http.StatusBadRequest)
		wrt.Write([]byte(err.Error() + "\r\n"))
		return
	}

	wrt.WriteHeader(http.StatusNoContent)
}

func (this *LogIngester) handleProcedure(req *http.Request) error {

	streamID := strings.ToLower(req.PathValue("id"))
	if streamID == "" {
		return errors.New("stream id required")
	}

	stream, has := this.Streams[streamID]
	if !has {
		slog.Warn("Ingester: Log stream not found",
			slog.String("id", streamID),
			slog.String("remote_addr", req.RemoteAddr))
		return errors.New("stream not found")
	}

	if stream.Token != "" {

		const bearerPrefix = "bearer"

		clientToken := req.Header.Get("authorization")
		if strings.HasPrefix(strings.ToLower(clientToken), bearerPrefix) {
			clientToken = strings.TrimSpace(clientToken[len(bearerPrefix):])
		}

		if clientToken != stream.Token {

			slog.Warn("Ingester: Stream token auth rejected",
				slog.String("id", streamID),
				slog.String("remote_addr", req.RemoteAddr))

			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			return errors.New("stream token auth failed")
		}
	}

	if size, err := strconv.Atoi(req.Header.Get("content-length")); err != nil {
		return errors.New("content-length required")
	} else if size > this.Cfg.MaxPayloadSize {
		return errors.New("content size too large")
	}

	contentType := req.Header.Get("content-type")
	switch {
	case strings.Contains(contentType, "json"):
		return this.handleJsonInput(&stream, req)
	default:
		return errors.New("unsupported content type")
	}
}

func (this *LogIngester) handleJsonInput(stream *StreamConfig, req *http.Request) error {

	type IngestedEntry struct {
		Date    int64         `json:"date"`
		Level   string        `json:"level"`
		Message string        `json:"message"`
		Meta    logs.Metadata `json:"meta"`
	}

	type IngestedPayload struct {
		Meta    logs.Metadata   `json:"meta"`
		Entries []IngestedEntry `json:"entries"`
	}

	var payload IngestedPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		slog.Debug("Ingester: Failed to parse payload",
			slog.String("err", err.Error()),
			slog.String("stream_id", stream.ID),
			slog.String("remote_addr", req.RemoteAddr))
		return errors.New("invalid batch payload")
	}

	if len(payload.Entries) == 0 {
		slog.Warn("Ingester: Empty payload",
			slog.String("stream_id", stream.ID),
			slog.String("remote_addr", req.RemoteAddr))
		return nil
	}

	slog.Debug("Received entries",
		slog.Int("count", len(payload.Entries)),
		slog.String("stream_id", stream.ID),
		slog.String("remote_addr", req.RemoteAddr))

	txID := uuid.New()

	batchLabels, batchMeta := splitMetaLabels(payload.Meta)

	var entries []logs.Entry
	for _, item := range payload.Entries {

		item.Message = strings.TrimSpace(item.Message)
		if item.Message == "" {
			continue
		}

		if item.Date <= 0 {
			item.Date = time.Now().UnixMilli()
		}

		next := logs.Entry{
			Time:      time.Unix(0, item.Date*int64(time.Millisecond)),
			StreamTag: stream.Tag,
			Level:     logs.Level(item.Level),
			Message:   truncateValue(item.Message, this.Cfg.MaxMessageSize),
			TxID:      null.StringFrom(txID.String()),
			Labels:    stream.Labels.Clone(),
			Meta:      item.Meta.Clone(),
		}

		if len(batchLabels) > 0 {
			batchLabels.CopyInto(&next.Labels)
		}

		if len(batchMeta) > 0 {
			batchMeta.CopyInto(&next.Meta)
		}

		labelFormat(next.Labels, this.Cfg)
		labelFormat(next.Meta, this.Cfg)

		entries = append(entries, next)
	}

	if len(entries) == 0 {
		slog.Warn("Ingester: Parsed result is empty",
			slog.String("stream_id", stream.ID),
			slog.String("remote_addr", req.RemoteAddr))
		return nil
	}

	slog.Debug("Ingest entries",
		slog.Int("count", len(entries)),
		slog.String("stream_id", stream.ID),
		slog.String("remote_addr", req.RemoteAddr))

	go func() {
		if err := this.Storage.Push(entries); err != nil {
			slog.Error("Ingest: storage push failed",
				slog.String("err", err.Error()),
				slog.String("stream_id", stream.ID),
				slog.String("remote_addr", req.RemoteAddr),
				slog.Int("entries", len(entries)))
		}
	}()

	return nil
}

func labelFormat(labels map[string]string, cfg IngesterConfig) {
	for key, val := range labels {

		keyFmt := truncateKey(stripLabel(strings.TrimSpace(key)), cfg.MaxLabelSize)
		valFmt := truncateValue(stripLabel(strings.TrimSpace(val)), cfg.MaxLabelSize)

		if valFmt == "" && cfg.KeepEmptyLabels {
			valFmt = "null"
		}

		if keyFmt == "" || valFmt == "" {
			delete(labels, key)
			continue
		}

		if keyFmt != key {
			delete(labels, key)
			labels[keyFmt] = valFmt
			key = keyFmt
		} else if valFmt != val {
			labels[key] = valFmt
		}
	}
}

func truncateValue(input string, n int) string {

	if len(input) < n {
		return input
	}

	return input[:n] + " ..."
}

func truncateKey(input string, n int) string {

	if len(input) < n {
		return input
	}

	return input[:n] + "___"
}

func stripLabel(key string) string {

	var stripped string

	for _, next := range key {

		switch {
		case next == '\\':
			stripped += "/"
		case unicode.IsPrint(next):
			stripped += string(next)
		default:
			stripped += "?"
		}
	}

	return stripped
}

func splitMetaLabels(labels logs.Metadata) (logs.Metadata, logs.Metadata) {

	if len(labels) == 0 {
		return nil, nil
	}

	setLabels := logs.Metadata{}
	setMeta := logs.Metadata{}

	for key, val := range labels {
		switch key {
		case "env", "environment":
			setLabels["env"] = val
		case "client_ip":
			setMeta["remote_addr"] = val
		default:
			setMeta[key] = val
		}
	}

	return setLabels, setMeta
}
