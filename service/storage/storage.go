package storage

import (
	"database/sql"
	"encoding/json"
	"maps"
	"time"

	"github.com/guregu/null"
)

type Storage interface {
	Push(entries []LogEntry) error
	QueryRange(from time.Time, to time.Time) ([]LogEntry, error)
	Close() error
}

type LogEntry struct {
	ID        null.Int
	Time      time.Time
	StreamTag string
	Level     Level
	Message   string
	Labels    Metadata
	Meta      Metadata
	TxID      null.String
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

func (this Metadata) Clone() Metadata {

	if this == nil {
		return nil
	}

	newMeta := Metadata{}
	maps.Copy(newMeta, this)
	return newMeta
}

func (this Metadata) CloneEntries() Metadata {

	if newMap := this.Clone(); newMap != nil {
		return newMap
	}

	return Metadata{}
}

func (this Metadata) CopyInto(dest *Metadata) {

	if *dest == nil {
		*dest = Metadata{}
	}

	maps.Copy(*dest, this)
}

func (this Metadata) ToNullBytes() sql.Null[[]byte] {

	if len(this) == 0 {
		return sql.Null[[]byte]{}
	}

	data, err := json.Marshal(this)
	if err != nil || this == nil {
		return sql.Null[[]byte]{}
	}

	return sql.Null[[]byte]{V: data, Valid: true}
}

func MetadataFromData(data sql.Null[[]byte]) Metadata {

	if !data.Valid {
		return nil
	}

	var meta Metadata
	if err := json.Unmarshal(data.V, &meta); err != nil {
		return nil
	}

	return meta
}
