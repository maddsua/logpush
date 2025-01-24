// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0

package dbops

import (
	"time"

	"github.com/google/uuid"
)

type Stream struct {
	ID        uuid.UUID
	CreatedAt time.Time
	Name      string
	Labels    []byte
}

type StreamEntry struct {
	ID        uuid.UUID
	CreatedAt time.Time
	StreamID  uuid.UUID
	TxID      uuid.NullUUID
	Level     string
	Message   string
	Metadata  []byte
}
