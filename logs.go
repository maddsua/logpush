package logpush

import (
	"context"
	"strings"
	"time"
)

type LogWriter interface {
	Type() string
	WriteEntry(ctx context.Context, entry LogEntry) error
	WriteBatch(ctx context.Context, batch []LogEntry) error
}

type LogEntry struct {
	//	Entry creation date
	Timestamp time.Time
	//	Unique log stream tag
	StreamTag string
	//	Log level (error|log|info|debug)
	LogLevel LogLevel
	//	The actual log message
	Message string
	//	Optional metadata in KV format
	Metadata map[string]string
}

type LogLevel string

func (this LogLevel) String() string {

	val := strings.ToLower(string(this))

	switch val {
	case "log", "warn", "error", "debug", "info", "trace":
		return val
	default:
		return "error"
	}
}
