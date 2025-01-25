package timescale

import (
	"database/sql"
	"encoding/json"
	"maps"
	"strings"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/ingester/streams"
	"github.com/maddsua/logpush/service/logdata"
)

func webStreamToRows(batch *streams.WebStream, streamID uuid.UUID, txID uuid.UUID) []dbops.InsertStreamEntryParams {

	var result []dbops.InsertStreamEntryParams

	var mergeMeta = func(entry streams.WebLogEntry) map[string]string {

		metadata := map[string]string{}

		maps.Copy(metadata, batch.Meta)
		logdata.CopyMetaFields(metadata, entry.Meta)

		if len(metadata) == 0 {
			return nil
		}

		return metadata
	}

	for idx, entry := range batch.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		var metadata sql.Null[[]byte]
		if meta := mergeMeta(entry); meta != nil {
			metadata.V, _ = json.Marshal(meta)
			metadata.Valid = true
		}

		result = append(result, dbops.InsertStreamEntryParams{
			CreatedAt: entry.Date.Time(idx),
			StreamID:  streamID,
			TxID:      uuid.NullUUID{UUID: txID, Valid: true},
			Level:     entry.Level.String(),
			Message:   entry.Message,
			Metadata:  metadata,
		})
	}

	return result
}
