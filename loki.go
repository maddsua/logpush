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

type LokiLabelTransformer func(val string) (newKey string, newValue string)

func lokiRenameLabel(newKey string) LokiLabelTransformer {
	return func(val string) (string, string) {
		return newKey, val
	}
}

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
		UseStructMeta: query.Get("labels") == "struct",
		ExtractLabels: map[string]LokiLabelTransformer{
			"level":       nil,
			"ip":          nil,
			"remote_addr": lokiRenameLabel("ip"),
			"client_ip":   lokiRenameLabel("ip"),
			"rid":         nil,
			"request_id":  lokiRenameLabel("rid"),
			"org":         nil,
			"app":         nil,
			"env":         nil,
			"environment": lokiRenameLabel("env"),
			"service":     nil,
			"scope":       nil,
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

	const attempts = 10
	const attemptDelay = 100 * time.Millisecond

	var doFetch = func() (*http.Response, error) {

		req, err := http.NewRequest(method, url.String(), body)
		if err != nil {
			return nil, err
		}

		for key, values := range headers {
			for _, val := range values {
				req.Header.Add(key, val)
			}
		}

		return http.DefaultClient.Do(req.WithContext(ctx))
	}

	var isOkayStatusCode = func(val int) bool {
		return val >= http.StatusOK && val <= http.StatusIMUsed
	}

	var doConsumeErrorResponse = func(resp *http.Response) {

		defer resp.Body.Close()

		switch resp.Header.Get("content-type") {

		case "application/json", "text/plain":
			if body, err := io.ReadAll(resp.Body); err == nil {
				slog.Debug("LOKI: API error",
					slog.Int("status", resp.StatusCode),
					slog.String("body", string(body)),
					slog.String("remote", this.baseURL.Host))
			}

		default:
			slog.Debug("LOKI: API error",
				slog.Int("status", resp.StatusCode),
				slog.String("remote", this.baseURL.Host))
		}
	}

	var lastErr error
	for idx := 0; idx < attempts && ctx.Err() == nil; idx++ {

		if resp, err := doFetch(); err != nil {
			slog.Debug("LOKI: API call failed",
				slog.String("err", err.Error()))
			lastErr = fmt.Errorf("http request: %v", err)
		} else if !isOkayStatusCode(resp.StatusCode) {

			doConsumeErrorResponse(resp)

			switch {

			//	retry on server errors
			case resp.StatusCode >= http.StatusInternalServerError:
				lastErr = fmt.Errorf("service down with status '%d'", resp.StatusCode)

			//	bail on client errors
			default:
				return nil, fmt.Errorf("unexpected status '%d'", resp.StatusCode)
			}

		} else {
			return resp, err
		}

		time.Sleep(attemptDelay)
	}

	return nil, lastErr
}

func (this *lokiWriter) Ping() error {

	const pingTimeout = 10 * time.Second

	pingUrl := this.baseURL
	pingUrl.Path = "/ready"

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

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

func (this *lokiWriter) WriteEntry(ctx context.Context, entry LogEntry) error {
	return this.WriteBatch(ctx, []LogEntry{entry})
}

func (this *lokiWriter) WriteBatch(ctx context.Context, batch []LogEntry) error {

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
						key, val = transform(val)
					}

					stream[key] = val
					continue
				}

				streamVal.StructuredMetadata[key] = val
			}
		}

		if entry.StreamTag != "" {
			stream["service_name"] = entry.StreamTag
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

	resp, err := this.fetch(ctx, http.MethodPost, pushUrl, headers, &body)
	if err == nil {
		defer resp.Body.Close()
	}

	return err
}
