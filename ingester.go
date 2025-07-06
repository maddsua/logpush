package logpush

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"math/rand"
)

type StreamConfig struct {
	Token  string            `yaml:"token" json:"token"`
	Labels map[string]string `yaml:"labels" json:"labels"`
}

type IngesterOptions struct {
	BasicAuth map[string]string `yaml:"basic_auth" json:"basic_auth"`

	MaxEntries int `yaml:"max_entries" json:"max_entries"`

	MaxMessageSize int `yaml:"max_message_size" json:"max_message_size"`

	MaxMetadataSize int `yaml:"max_metadata_size" json:"max_metadata_size"`
	MaxLabelSize    int `yaml:"max_label_size" json:"max_label_size"`
	MaxFieldSize    int `yaml:"max_field_size" json:"max_field_size"`
}

type LogIngester struct {
	Writer  LogWriter
	Options IngesterOptions
	Streams map[string]StreamConfig

	optionsValid bool
}

func (this *LogIngester) validateOptions() {

	if this.Options.MaxEntries <= 0 {
		this.Options.MaxEntries = 1024
	}

	if this.Options.MaxMessageSize <= 0 {
		this.Options.MaxMessageSize = 16 * 1024
	}

	if this.Options.MaxLabelSize <= 0 {
		this.Options.MaxLabelSize = 64
	}

	if this.Options.MaxFieldSize <= 0 {
		this.Options.MaxFieldSize = 1024
	}

	if this.Options.MaxMetadataSize <= 64 {
		this.Options.MaxMetadataSize = 16 * 1024
	}

	this.optionsValid = true
}

func (this *LogIngester) ServeHTTP(wrt http.ResponseWriter, req *http.Request) {

	if !this.optionsValid {
		this.validateOptions()
	}

	clientIP := parseXff(req)

	var respondError = func(message string, status int) {

		if status < http.StatusOK {
			status = http.StatusBadRequest
		}

		slog.Error("INGESTER http request",
			slog.String("ip", clientIP),
			slog.String("err", message))

		wrt.Header().Set("content-type", "text/plain")
		wrt.WriteHeader(http.StatusBadRequest)
		wrt.Write([]byte(message + "\r\n"))
	}

	if this.Writer == nil {
		respondError("no available writer", http.StatusInternalServerError)
		return
	}

	if len(this.Options.BasicAuth) > 0 {

		if user, pass, has := req.BasicAuth(); !has {
			respondError("authorization required", http.StatusUnauthorized)
			return
		} else if expectPass, hasUser := this.Options.BasicAuth[user]; !hasUser || pass != expectPass {
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			respondError("invalid credentials", http.StatusForbidden)
			return
		}
	}

	streamID := strings.ToLower(req.PathValue("id"))
	if streamID == "" {
		respondError("stream id required", http.StatusBadRequest)
		return
	}

	stream, has := this.Streams[streamID]
	if !has {
		respondError(fmt.Sprintf("stream '%s' not found", streamID), http.StatusNotFound)
		return
	}

	if stream.Token != "" {

		const bearerPrefix = "bearer"

		clientToken := req.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(clientToken), bearerPrefix) {
			clientToken = strings.TrimSpace(clientToken[len(bearerPrefix):])
		}

		if clientToken != stream.Token {
			time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
			respondError(fmt.Sprintf("auth token rejected for stream '%s'", streamID), http.StatusForbidden)
			return
		}
	}

	contentType := req.Header.Get("content-type")
	switch {

	case strings.Contains(contentType, "json"):

		var batch IngesterBatch
		if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
			respondError(fmt.Sprintf("failed to decode batch: %v", err), http.StatusBadRequest)
			return
		}

		if len(batch.Entries) == 0 {
			slog.Warn("INGESTER Empty payload",
				slog.String("ip", clientIP),
				slog.String("stream_id", streamID))
			break
		}

		slog.Debug("INGESTER Received",
			slog.Int("entries", len(batch.Entries)),
			slog.String("ip", clientIP),
			slog.String("stream_id", streamID))

		if this.Options.MaxEntries > 0 && len(batch.Entries) > this.Options.MaxEntries {
			slog.Warn("INGESTER Entries truncated",
				slog.Int("entries", len(batch.Entries)),
				slog.Int("trunc", this.Options.MaxEntries),
				slog.String("ip", clientIP),
				slog.String("stream_id", streamID))
			batch.Entries = batch.Entries[:this.Options.MaxEntries]
		}

		var entries []LogEntry

		for _, entry := range batch.Entries {

			var totalMetadataSize int
			meta := map[string]string{}

			var canAddField = func(key string, val string) bool {
				totalMetadataSize += len(key) + len(val)
				return totalMetadataSize < this.Options.MaxMetadataSize
			}

			//	index batch labels first without adding them
			for key, val := range batch.Meta {
				_ = canAddField(key, val)
			}

			//	copy entry labels if still have space left
			for key, val := range entry.Meta {
				if canAddField(key, val) {
					meta[stripLabel(truncateKey(key, this.Options.MaxLabelSize))] = stripLabel(truncateValue(val, this.Options.MaxFieldSize))
				}
			}

			//	write batch labels over everything else
			for key, val := range batch.Meta {
				meta[stripLabel(truncateKey(key, this.Options.MaxLabelSize))] = stripLabel(truncateValue(val, this.Options.MaxFieldSize))
			}

			var timestamp time.Time
			if entry.Date >= 0 {
				timestamp = time.Unix(0, entry.Date*int64(time.Millisecond))
			} else {
				timestamp = time.Now()
			}

			if this.Options.MaxMessageSize > 0 && len(entry.Message) > this.Options.MaxMessageSize {
				slog.Warn("INGESTER Message truncated",
					slog.Int("len", len(entry.Message)),
					slog.Int("trunc", this.Options.MaxMessageSize),
					slog.String("ip", clientIP),
					slog.String("stream_id", streamID))
				entry.Message = entry.Message[:this.Options.MaxMessageSize] + "..."
			}

			entries = append(entries, LogEntry{
				Timestamp: timestamp,
				StreamTag: streamID,
				LogLevel:  LogLevel(entry.Level),
				Message:   entry.Message,
				Metadata:  meta,
			})
		}

		go func() {
			if err := this.Writer.WriteBatch(context.Background(), entries); err != nil {
				slog.Error("INGESTER Writer.WriteBatch",
					slog.String("writer_type", this.Writer.Type()),
					slog.String("err", err.Error()))
			}
		}()

	default:
		respondError("unsupported content type", http.StatusNotAcceptable)
		return
	}

	wrt.WriteHeader(http.StatusNoContent)
}

func parseXff(req *http.Request) string {
	if xff := req.Header.Get("x-forwarded-for"); xff != "" {
		return xff
	} else if host, _, _ := net.SplitHostPort(req.RemoteAddr); host != "" {
		return host
	}
	return req.RemoteAddr
}

type IngesterBatch struct {
	Meta    map[string]string `json:"meta"`
	Entries []IngesterEntry   `json:"entries"`
}

type IngesterEntry struct {
	Date    int64             `json:"date"`
	Level   string            `json:"level"`
	Message string            `json:"message"`
	Meta    map[string]string `json:"meta"`
}

func stripLabel(val string) string {

	var stripped string
	for _, next := range val {

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

func truncateValue(val string, n int) string {

	if n <= 0 {
		return val
	}

	if len(val) < n {
		return val
	}

	return val[:n] + " ..."
}

func truncateKey(val string, n int) string {

	if n <= 0 {
		return val
	}

	if len(val) < n {
		return val
	}

	return val[:n] + "___"
}
