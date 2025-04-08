package loki

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/maddsua/logpush/service/logs"
)

func NewLokiStorage(urlstring string) (*Loki, error) {

	parsed, err := url.Parse(urlstring)
	if err != nil {
		return nil, err
	}

	if parsed.Scheme == "" {
		return nil, fmt.Errorf("url scheme must be http or https")
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("url host is not defined")
	}

	loki := &Loki{url: parsed}

	if err := loki.ready(); err != nil {
		return nil, fmt.Errorf("loki connection down: %s", err.Error())
	}

	return loki, nil
}

type Loki struct {
	url *url.URL
}

func (this *Loki) Close() error {
	return nil
}

func (this *Loki) ready() error {

	useUrl := copyBaseUrl(this.url)
	useUrl.Path = "/ready"

	req, err := http.NewRequest("GET", useUrl.String(), nil)
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

	return fmt.Errorf("[http] %d", resp.StatusCode)
}

func (this *Loki) QueryRange(from time.Time, to time.Time) ([]logs.Entry, error) {
	return nil, errors.New("loki storage doesn't support reads currently")
}

func (this *Loki) Push(entries []logs.Entry) error {

	if len(entries) == 0 {
		return nil
	}

	type LogValue []any

	type StreamEntry struct {
		Stream map[string]string `json:"stream"`
		Values []LogValue        `json:"values"`
	}

	type PostPayload struct {
		Streams []StreamEntry `json:"streams"`
	}

	useUrl := copyBaseUrl(this.url)
	useUrl.Path = "/loki/api/v1/push"

	var streams []StreamEntry

	for idx := 0; idx < len(entries); idx++ {

		var next StreamEntry

		if len(entries[idx].Labels) > 0 {
			next.Stream = entries[idx].Labels
		} else {
			next.Stream = logs.Metadata{}
		}

		for ; idx < len(entries); idx++ {

			entry := entries[idx]

			if !compareMetadata(entry.Labels, next.Stream) {
				break
			}

			meta := entry.Meta.CloneEntries()
			meta["level"] = entry.Level.String()
			meta["service"] = entry.StreamTag

			if entry.TxID.Valid {
				meta["logpush_tx"] = entry.TxID.String
			}

			next.Values = append(next.Values, LogValue{
				timeFmt(entry.Time, idx),
				entry.Message,
				meta,
			})
		}

		next.Stream["logpush_source"] = "web"

		streams = append(streams, next)
	}

	postBody, err := json.Marshal(PostPayload{
		Streams: streams,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", useUrl.String(), bytes.NewReader(postBody))
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

	switch resp.Header.Get("content-type") {
	case "application/json", "text/plain":
		return readErrorText(req.Body)
	default:
		return fmt.Errorf("failed to push log streams: [http] %d", resp.StatusCode)
	}
}

func readErrorText(reader io.Reader) error {

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read error response body: %s", err.Error())
	}

	return fmt.Errorf("failed to push log streams: %s", string(data))
}

func compareMetadata(a logs.Metadata, b logs.Metadata) bool {

	if len(a) != len(b) {
		return false
	}

	if (a == nil && b != nil) || (b == nil && a != nil) {
		return false
	}

	for key, val := range a {
		if b[key] != val {
			return false
		}
	}

	return true
}

func timeFmt(date time.Time, sequence int) string {
	return strconv.FormatInt(date.UnixNano()+int64(sequence), 10)
}

func copyBaseUrl(base *url.URL) *url.URL {
	return &url.URL{
		Scheme: base.Scheme,
		Host:   base.Host,
		User:   base.User,
	}
}
