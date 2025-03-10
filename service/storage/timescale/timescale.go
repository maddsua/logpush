package timescale

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/guregu/null"
	_ "github.com/lib/pq"
	"github.com/maddsua/logpush/service/storage"
	"github.com/maddsua/logpush/service/storage/timescale/queries"

	"github.com/golang-migrate/migrate/v4"
	postgres_migrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var migfs embed.FS

func NewTimescaleStorage(dbUrl string) (*timescaleStorage, error) {

	connUrl, err := url.Parse(dbUrl)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		return nil, err
	}

	slog.Debug("Storage: Timescale enabled",
		slog.String("host", connUrl.Host),
		slog.String("name", strings.TrimPrefix(connUrl.Path, "/")))

	storage := &timescaleStorage{db: db, queries: queries.New(db)}

	if err := storage.migrate(db); err != nil {
		return nil, fmt.Errorf("failed to run storage migrations: %s", err.Error())
	}

	return storage, nil
}

type timescaleStorage struct {
	db      *sql.DB
	queries *queries.Queries
}

func (this *timescaleStorage) Close() error {
	return this.db.Close()
}

func (this *timescaleStorage) Push(entries []storage.LogEntry) error {

	tx, err := this.db.Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	for _, item := range entries {
		if err := this.queries.InsertEntry(context.Background(), queries.InsertEntryParams{
			Time:      item.Time,
			Level:     item.Level.String(),
			Message:   item.Message,
			Labels:    item.Labels.ToNullBytes(),
			Meta:      item.Meta.ToNullBytes(),
			TxID:      item.TxID.NullString,
			StreamTag: item.StreamTag,
		}); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (this *timescaleStorage) QueryRange(from time.Time, to time.Time) ([]storage.LogEntry, error) {

	entries, err := this.queries.GetEntriesRange(context.Background(), queries.GetEntriesRangeParams{
		RangeFrom: from,
		RangeTo:   to,
	})
	if err != nil {
		return nil, err
	}

	result := make([]storage.LogEntry, len(entries))
	for idx, val := range entries {
		result[idx] = storage.LogEntry{
			ID:        null.IntFrom(val.ID),
			Time:      val.Time,
			Message:   val.Message,
			Level:     storage.Level(val.Level),
			Labels:    storage.MetadataFromData(val.Labels),
			Meta:      storage.MetadataFromData(val.Meta),
			StreamTag: val.StreamTag,
			TxID:      null.String{NullString: val.TxID},
		}
	}

	return result, nil
}

func (this *timescaleStorage) migrate(db *sql.DB) error {

	migfs, err := iofs.New(migfs, "migrations")
	if err != nil {
		return err
	}

	migdb, err := postgres_migrate.WithInstance(db, &postgres_migrate.Config{})
	if err != nil {
		return err
	}

	mig, err := migrate.NewWithInstance("iofs", migfs, "postgresql", migdb)
	if err != nil {
		return err
	}

	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	version, ditry, err := mig.Version()
	if err != nil {
		return err
	}

	slog.Debug("Storage migrated",
		slog.Int("version", int(version)),
		slog.Bool("dirty", ditry))

	return nil
}
