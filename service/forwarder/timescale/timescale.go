package timescale

import (
	"context"
	"database/sql"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/ingester/streams"
)

type Timescale struct {
	DB *sql.DB
}

func (this *Timescale) IngestWeb(streamSource *dbops.Stream, txID uuid.UUID, remoteAddr string, payload *streams.WebStream) {

	rows := webStreamToRows(payload, streamSource.ID, txID)
	if len(rows) == 0 {
		slog.Warn("LOKI FORWARDER: Empty log batch",
			slog.String("stream_id", streamSource.ID.String()),
			slog.String("remote_addr", remoteAddr))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tx, err := this.DB.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("TIMESCALE FORWARDER: Failed to begin DB TX",
			slog.String("err", err.Error()),
			slog.String("stream_id", streamSource.ID.String()),
			slog.String("remote_addr", remoteAddr))
		return
	}

	txq := dbops.New(tx)

	for _, row := range rows {
		if err := txq.InsertStreamEntry(ctx, row); err != nil {
			slog.Error("TIMESCALE FORWARDER: Failed to insert row",
				slog.String("err", err.Error()),
				slog.String("stream_id", streamSource.ID.String()),
				slog.String("remote_addr", remoteAddr))
		}
	}

	defer tx.Rollback()

	if err := tx.Commit(); err != nil {
		slog.Error("TIMESCALE FORWARDER: Failed to commit DB TX",
			slog.String("err", err.Error()),
			slog.String("stream_id", streamSource.ID.String()),
			slog.String("remote_addr", remoteAddr))
		return
	}

	slog.Debug("TIMESCALE FORWARDER: Wrote entries",
		slog.Int("count", len(rows)),
		slog.String("stream_id", streamSource.ID.String()),
		slog.String("remote_addr", remoteAddr))
}
