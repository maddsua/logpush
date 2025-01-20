package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
)

const webStreamPushRetryAttempts = 3

type LogIngester struct {
	DB          *dbops.Queries
	Loki        *LokiConnection
	Timescale   *Timescale
	StreamCache *StreamCache
}

func (this *LogIngester) ServeHTTP(writer http.ResponseWriter, req *http.Request) {

	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		req.RemoteAddr = xff
	} else if host, _, _ := net.SplitHostPort(req.RemoteAddr); host != "" {
		req.RemoteAddr = host
	}

	if err := this.handleRequest(req); err != nil {
		writer.Header().Set("content-type", "text/plain")
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write([]byte(err.Error() + "\r\n"))
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func (this *LogIngester) handleRequest(req *http.Request) error {

	serviceID, err := uuid.Parse(req.PathValue("id"))
	if err != nil {
		return errors.New("service id required")
	}

	var logStream dbops.Stream

	if cached := this.StreamCache.Get(serviceID); cached != nil {

		if cached.Entry == nil {
			return errors.New("service not found")
		}

		logStream = *cached.Entry

	} else {

		if logStream, err = this.DB.GetStream(req.Context(), serviceID); err != nil {

			if err == sql.ErrNoRows {
				this.StreamCache.Set(serviceID, nil)
				return errors.New("service not found")
			}

			slog.Error("Failed to query log stream",
				slog.String("err", err.Error()))

			return errors.New("unable to query requested service stream")
		}

		this.StreamCache.Set(serviceID, &logStream)
	}

	contentType := req.Header.Get("content-type")
	switch {
	case strings.Contains(contentType, "json"):

		var payload WebStream
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			return errors.New("invalid batch payload")
		}

		slog.Info("WEB STREAM: Ingesting entries",
			slog.Int("count", len(payload.Entries)),
			slog.String("stream_id", logStream.ID.String()),
			slog.String("remote_addr", req.RemoteAddr))

		ingester := this.Timescale.IngestWeb
		if this.Loki != nil {
			ingester = this.Loki.IngestWeb
		}

		go ingester(IngestSource{
			Stream:     logStream,
			RemoteAddr: req.RemoteAddr,
		}, payload)

		return nil
	default:
		return errors.New("invalid content type")
	}
}

type IngestSource struct {
	Stream     dbops.Stream
	RemoteAddr string
}

type WebStream struct {
	ServiceID string            `json:"service_id"`
	Meta      map[string]string `json:"meta"`
	Entries   []WebLogEntry     `json:"entries"`
}

type WebLogEntry struct {
	Date    UnixMilli         `json:"date"`
	Level   Level             `json:"level"`
	Message string            `json:"message"`
	Meta    map[string]string `json:"meta"`
}

func (batch *WebStream) ToLokiStream(streamName string) LokiStream {

	labels := map[string]string{
		"source":       "web",
		"service_name": streamName,
	}

	metaFields := map[string]string{}

	for key, val := range batch.Meta {
		switch key {
		case "service_name", "source":
			continue
		case "env", "environment":
			labels["env"] = val
		case "request_id", "transaction_id", "rid", "tx_id":
			labels["request_id"] = val
		default:
			metaFields[key] = val
		}
	}

	if _, ok := labels["request_id"]; !ok {
		labels["request_id"] = uuid.New().String()
	}

	var streamValues [][]any
	for idx, entry := range batch.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		meta := map[string]string{}
		maps.Copy(meta, metaFields)
		meta["detected_level"] = entry.Level.String()

		if entry.Meta != nil {
			maps.Copy(meta, entry.Meta)
		}

		streamValues = append(streamValues, []any{
			entry.Date.String(idx),
			entry.Message,
			meta,
		})
	}

	return LokiStream{
		Stream: labels,
		Values: streamValues,
	}
}

func (batch *WebStream) ToTimescaleRows(streamID uuid.UUID) []dbops.InsertStreamEntryParams {

	var result []dbops.InsertStreamEntryParams

	var copyFields = func(src map[string]string, dst map[string]string) {

		if src == nil || dst == nil {
			return
		}

		for key, val := range src {
			if mval, has := dst[key]; has {
				dst["_entry_"+key] = mval
			}
			dst[key] = val
		}
	}

	var mergeMeta = func(entry WebLogEntry) map[string]string {

		metadata := map[string]string{}

		copyFields(batch.Meta, metadata)
		copyFields(entry.Meta, metadata)

		if len(metadata) == 0 {
			return nil
		}

		return metadata
	}

	for idx, entry := range batch.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		var metadata json.RawMessage
		if meta := mergeMeta(entry); meta != nil {
			metadata, _ = json.Marshal(meta)
		}

		result = append(result, dbops.InsertStreamEntryParams{
			StreamID:  streamID,
			CreatedAt: entry.Date.Time(idx),
			Level:     entry.Level.String(),
			Message:   entry.Message,
			Metadata:  metadata,
		})
	}

	return result
}

func NewStreamCache() *StreamCache {
	return &StreamCache{
		data:        map[uuid.UUID]StreamCacheEntry{},
		nextCleanup: time.Now().Add(time.Minute),
	}
}

type StreamCache struct {
	data        map[uuid.UUID]StreamCacheEntry
	mtx         sync.Mutex
	nextCleanup time.Time
}

type StreamCacheEntry struct {
	Entry   *dbops.Stream
	Expires time.Time
}

func (this *StreamCache) Set(key uuid.UUID, entry *dbops.Stream) {

	this.mtx.Lock()
	defer this.mtx.Unlock()

	this.data[key] = StreamCacheEntry{
		Entry:   entry,
		Expires: time.Now().Add(time.Minute),
	}
}

func (this *StreamCache) Get(key uuid.UUID) *StreamCacheEntry {

	this.mtx.Lock()
	defer this.mtx.Unlock()

	now := time.Now()

	if this.nextCleanup.Before(now) {

		for key, val := range this.data {
			if val.Expires.Before(now) {
				delete(this.data, key)
			}
		}

		this.nextCleanup = now.Add(time.Minute)
	}

	if entry, has := this.data[key]; has {
		return &entry
	}

	return nil
}
