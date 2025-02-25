package main

import (
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/guregu/null"
	"github.com/maddsua/logpush/service/storage"
)

type LogIngester struct {
	Storage storage.Storage
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
			slog.String("id", streamID))
		return errors.New("stream not found")
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
		Date    int64             `json:"date"`
		Level   string            `json:"level"`
		Message string            `json:"message"`
		Meta    map[string]string `json:"meta"`
	}

	type IngestedPayload struct {
		Meta    map[string]string `json:"meta"`
		Entries []IngestedEntry   `json:"entries"`
	}

	var payload IngestedPayload
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		slog.Debug("Ingester: Failed to parse payload",
			slog.String("err", err.Error()),
			slog.String("stream_id", stream.ID),
			slog.String("remote_addr", req.RemoteAddr))
		return errors.New("invalid batch payload")
	}

	slog.Debug("Received entries",
		slog.Int("count", len(payload.Entries)),
		slog.String("stream_id", stream.ID),
		slog.String("remote_addr", req.RemoteAddr))

	txID := uuid.New()

	var entries []storage.LogEntry
	for _, item := range payload.Entries {

		item.Message = strings.TrimSpace(item.Message)
		if item.Message == "" {
			continue
		}

		if item.Date <= 0 {
			item.Date = time.Now().UnixMilli()
		}

		next := storage.LogEntry{
			Time:    time.Unix(0, item.Date*int64(time.Millisecond)),
			Level:   storage.Level(item.Level),
			Message: item.Message,
			TxID:    null.StringFrom(txID.String()),
		}

		if len(stream.Name) > 0 {
			next.ServiceName = null.StringFrom(stream.Name)
		}

		if len(item.Meta) > 0 {
			next.Meta = item.Meta
		}

		if len(stream.Labels) > 0 {
			next.Labels = maps.Clone(stream.Labels)
		}

		if len(payload.Meta) > 0 {
			if next.Labels == nil {
				next.Labels = maps.Clone(payload.Meta)
			} else {
				maps.Copy(next.Labels, payload.Meta)
			}
		}

		labelCleanup(next.Labels, this.Cfg)
		labelCleanup(next.Meta, this.Cfg)

		entries = append(entries, next)
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

func labelCleanup(labels map[string]string, cfg IngesterConfig) {
	for key, val := range labels {

		cleanVal := strings.TrimSpace(val)
		cleanKey := strings.TrimSpace(key)

		if cleanVal == "" && cfg.KeepEmptyLabels {
			cleanVal = "null"
		}

		if cleanKey == "" || cleanVal == "" {
			delete(labels, key)
			continue
		}

		if cleanKey != key {
			labels[cleanKey] = cleanVal
			delete(labels, key)
			continue
		}
	}
}
