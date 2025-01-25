package streams

import "github.com/maddsua/logpush/service/logdata"

type WebStream struct {
	ServiceID string            `json:"service_id"`
	Meta      map[string]string `json:"meta"`
	Entries   []WebLogEntry     `json:"entries"`
}

type WebLogEntry struct {
	Date    logdata.UnixMilli `json:"date"`
	Level   logdata.Level     `json:"level"`
	Message string            `json:"message"`
	Meta    map[string]string `json:"meta"`
}
