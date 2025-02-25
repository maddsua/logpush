package storage

import (
	"encoding/json"
	"time"

	"github.com/guregu/null"
)

type Storage interface {
	Store(entries []LogEntry) error
	Close() error
}

type LogEntry struct {
	Time        time.Time
	Level       Level
	Message     string
	Labels      Metadata
	Meta        Metadata
	TxID        null.String
	ServiceName string
}

type Level string

func (lvl Level) String() string {
	switch lvl {
	case "log", "warn", "error", "debug", "info":
		return string(lvl)
	default:
		return "error"
	}
}

type Metadata map[string]string

func (this *Metadata) Data() []byte {

	if len(*this) == 0 {
		return nil
	}

	data, err := json.Marshal(this)
	if err != nil || this == nil {
		return []byte("{}")
	}

	return data
}

func (this *Metadata) FromData(data []byte) error {
	return json.Unmarshal(data, this)
}
