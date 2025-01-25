package ingester

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/forwarder/loki"
	"github.com/maddsua/logpush/service/forwarder/timescale"
	"github.com/maddsua/logpush/service/ingester/streams"
)

type Ingester struct {
	DB          *dbops.Queries
	Loki        *loki.Loki
	Timescale   *timescale.Timescale
	StreamCache *StreamCache
}

func (this *Ingester) ServeHTTP(writer http.ResponseWriter, req *http.Request) {

	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		req.RemoteAddr = xff
	} else if host, _, _ := net.SplitHostPort(req.RemoteAddr); host != "" {
		req.RemoteAddr = host
	}

	if err := this.HandleRequest(req); err != nil {
		writer.Header().Set("content-type", "text/plain")
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write([]byte(err.Error() + "\r\n"))
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func (this *Ingester) HandleRequest(req *http.Request) error {

	streamID, err := uuid.Parse(req.PathValue("id"))
	if err != nil {
		return errors.New("service id required")
	}

	var getLogStream = func() (*dbops.Stream, error) {

		if cached := this.StreamCache.Get(streamID); cached != nil {
			return cached.Entry, nil
		}

		entry, err := this.DB.GetStream(req.Context(), streamID)
		if err != nil {

			if err == sql.ErrNoRows {
				this.StreamCache.Set(streamID, nil)
				return nil, nil
			}

			return nil, err
		}

		this.StreamCache.Set(streamID, &entry)
		return &entry, nil
	}

	logStream, err := getLogStream()
	if err != nil {
		slog.Error("WEB STREAM: Failed to query log stream",
			slog.String("err", err.Error()))
		return errors.New("unable to query requested service stream")
	} else if logStream == nil {
		slog.Warn("WEB STREAM: Log stream not found",
			slog.String("id", streamID.String()))
		return errors.New("service not found")
	}

	contentType := req.Header.Get("content-type")
	switch {
	case strings.Contains(contentType, "json"):

		var payload streams.WebStream
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			slog.Debug("WEB STREAM: Failed to parse payload",
				slog.String("err", err.Error()),
				slog.String("stream_id", logStream.ID.String()),
				slog.String("remote_addr", req.RemoteAddr))
			return errors.New("invalid batch payload")
		}

		slog.Debug("WEB STREAM: Ingesting entries",
			slog.Int("count", len(payload.Entries)),
			slog.String("stream_id", logStream.ID.String()),
			slog.String("remote_addr", req.RemoteAddr))

		txID := uuid.New()

		if this.Loki != nil {
			go this.Loki.IngestWeb(logStream, txID, req.RemoteAddr, &payload)
		} else {
			go this.Timescale.IngestWeb(logStream, txID, req.RemoteAddr, &payload)
		}

		return nil
	default:
		return errors.New("invalid content type")
	}
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
