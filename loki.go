package logpush

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values []LokiStreamValue `json:"values"`
}

type LokiStreamValue struct {
	Sequence           int64
	Timestamp          time.Time
	LogLine            string
	StructuredMetadata map[string]string
}

func (this LokiStreamValue) MarshalJSON() ([]byte, error) {

	line := []any{
		strconv.FormatInt(this.Timestamp.UnixNano()+this.Sequence, 10),
		this.LogLine,
	}

	if len(this.StructuredMetadata) > 0 {
		line = append(line, this.StructuredMetadata)
	}

	return json.Marshal(line)
}

type LokiLabelTransformer func(val string) string

func NewLokiWriter(lokiUrl string) (*lokiWriter, error) {

	baseURL, err := url.Parse(lokiUrl)
	if err != nil {
		return nil, err
	}

	if baseURL.Host == "" {
		return nil, fmt.Errorf("url host is not defined")
	}

	switch baseURL.Scheme {
	case "":
		baseURL.Scheme = "http"
	case "http", "https":
		break
	default:
		return nil, fmt.Errorf("unsupported url protocol")
	}

	query := baseURL.Query()

	this := lokiWriter{
		baseURL: url.URL{
			Scheme: baseURL.Scheme,
			Host:   baseURL.Host,
			User:   baseURL.User,
		},
		UseStructMeta: query.Get("labels") == "struct" || query.Get("s_meta") == "true",
		ExtractLabels: map[string]LokiLabelTransformer{
			"level":       nil,
			"remote_addr": nil,
			"client_ip":   nil,
			"ip":          nil,
			"org":         nil,
			"app":         nil,
			"env":         nil,
			"api":         nil,
			"scope":       nil,
			"request_id":  nil,
		},
	}

	if err := this.Ping(); err != nil {
		return nil, fmt.Errorf("unable to connect: %s", err.Error())
	}

	return &this, err
}

type lokiWriter struct {
	baseURL url.URL

	//	Use structured metadata
	UseStructMeta bool

	//	Extract these labels when structured metadata is enabled
	ExtractLabels map[string]LokiLabelTransformer
}

func (this *lokiWriter) Type() string {
	return "loki"
}

func (this *lokiWriter) fetch(ctx context.Context, method string, url url.URL, headers http.Header, body io.Reader) (*http.Response, error) {

	req, err := http.NewRequest(method, url.String(), body)
	if err != nil {
		return nil, err
	}

	for key, values := range headers {
		for _, val := range values {
			req.Header.Add(key, val)
		}
	}

	resp, err := http.DefaultClient.Do(req.WithContext(ctx))

	if err == nil {

		switch resp.Header.Get("content-type") {

		case "application/json", "text/plain":
			if body, err := io.ReadAll(resp.Body); err == nil {
				slog.Debug("LOKI: API error",
					slog.Int("status", resp.StatusCode),
					slog.String("body", string(body)))
			}
		}

	} else {
		slog.Debug("LOKI: API error",
			slog.String("err", err.Error()))
	}

	return resp, err
}

func (this *lokiWriter) Ping() error {

	const attempts = 10
	const timeout = 10 * time.Second

	pingUrl := this.baseURL
	pingUrl.Path = "/ready"

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var doPing = func() error {

		resp, err := this.fetch(ctx, http.MethodGet, pingUrl, nil, nil)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			return fmt.Errorf("unexpected status '%d'", resp.StatusCode)
		}

		return nil
	}

	var lastErr error
	for idx := 0; idx < attempts && ctx.Err() == nil; idx++ {
		if lastErr = doPing(); lastErr == nil {
			return nil
		}
	}

	return lastErr
}

func (this *lokiWriter) WriteEntry(ctx context.Context, entry LogEntry) error {
	return this.WriteBatch(ctx, []LogEntry{entry})
}

func (this *lokiWriter) WriteBatch(ctx context.Context, batch []LogEntry) error {

	const attempts = 10

	pushUrl := this.baseURL
	pushUrl.Path = "/loki/api/v1/push"

	var streams []LokiStream

	for idx, entry := range batch {

		stream := map[string]string{}

		streamVal := LokiStreamValue{
			Sequence:  int64(idx),
			Timestamp: entry.Timestamp,
			LogLine:   entry.Message,
		}

		if !this.UseStructMeta {

			for key, val := range entry.Metadata {
				stream[key] = val
			}

		} else {

			streamVal.StructuredMetadata = map[string]string{}

			for key, val := range entry.Metadata {

				if transform, isLabel := this.ExtractLabels[key]; isLabel {

					if transform != nil {
						val = transform(val)
					}

					stream[key] = val
					continue
				}

				streamVal.StructuredMetadata[key] = val
			}
		}

		if entry.StreamTag != "" {
			stream["stream_tag"] = entry.StreamTag
		}

		stream["level"] = entry.LogLevel.String()
		stream["mws_source"] = "logpush"

		streams = append(streams, LokiStream{Stream: stream, Values: []LokiStreamValue{streamVal}})
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(map[string]any{
		"streams": streams,
	}); err != nil {
		return fmt.Errorf("json.Marshal: %v", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")

	var doPost = func() (error, bool) {

		resp, err := this.fetch(ctx, http.MethodPost, pushUrl, headers, &body)
		if err != nil {
			return err, true
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 300 {
			return fmt.Errorf("unexpected status '%d'", resp.StatusCode), false
		}

		return nil, false
	}

	var lastErr error
	for idx := 0; idx < attempts && ctx.Err() == nil; idx++ {

		err, retriable := doPost()
		if err == nil {
			return nil
		} else if !retriable {
			return err
		}

		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}

	return lastErr
}
