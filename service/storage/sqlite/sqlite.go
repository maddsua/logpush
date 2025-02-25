package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/guregu/null"
	"github.com/maddsua/logpush/service/storage"
	"github.com/maddsua/logpush/service/storage/sqlite/queries"
	_ "github.com/mattn/go-sqlite3"

	"github.com/golang-migrate/migrate/v4"
	sqlite_migrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var migfs embed.FS

func NewSqliteStorage(path string) (*sqliteStorage, error) {

	params := url.Values{}

	if before, after, has := strings.Cut(path, "?"); has {

		query, err := url.ParseQuery(after)
		if err != nil {
			return nil, err
		}

		path = before
		params = query
	}

	params.Set("_journal", "WAL")

	switch {
	case strings.HasSuffix(path, ".db"), strings.HasSuffix(path, ".db3"):
	default:
		path = filepath.Join(path, "./storage.db3")
	}

	if dir := filepath.Dir(path); dir != "." && dir != "/" && dir != "\\" {
		if _, err := os.Stat(dir); err != nil {
			if err := os.Mkdir(dir, os.ModePerm); err != nil {
				return nil, err
			}
		}
	}

	if len(params) > 0 {
		path = path + "?" + params.Encode()
	}

	slog.Debug("Storage: sqlite3 enabled",
		slog.String("path", path))

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	storage := &sqliteStorage{db: db, queries: queries.New(db)}

	if err := storage.migrate(db); err != nil {
		return nil, fmt.Errorf("failed to run storage migrations: %s", err.Error())
	}

	return storage, nil
}

type sqliteStorage struct {
	db      *sql.DB
	queries *queries.Queries
}

func (this *sqliteStorage) Close() error {
	return this.db.Close()
}

func (this *sqliteStorage) Push(entries []storage.LogEntry) error {

	tx, err := this.db.Begin()
	if err != nil {
		return err
	}

	defer tx.Rollback()

	for _, item := range entries {
		if err := this.queries.InsertEntry(context.Background(), queries.InsertEntryParams{
			Time:      item.Time.UnixNano(),
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

func (this *sqliteStorage) QueryRange(from time.Time, to time.Time) ([]storage.LogEntry, error) {

	entries, err := this.queries.GetEntriesRange(context.Background(), queries.GetEntriesRangeParams{
		RangeFrom: from.UnixNano(),
		RangeTo:   to.UnixNano(),
	})
	if err != nil {
		return nil, err
	}

	result := make([]storage.LogEntry, len(entries))
	for idx, item := range entries {
		result[idx] = storage.LogEntry{
			ID:        null.IntFrom(item.ID),
			Time:      time.Unix(0, item.Time),
			Message:   item.Message,
			Level:     storage.Level(item.Level),
			Labels:    storage.MetadataFromData(item.Labels),
			Meta:      storage.MetadataFromData(item.Meta),
			StreamTag: item.StreamTag,
			TxID:      null.String{NullString: item.TxID},
		}
	}

	return result, nil
}

func (this *sqliteStorage) migrate(db *sql.DB) error {

	migfs, err := iofs.New(migfs, "migrations")
	if err != nil {
		return err
	}

	migdb, err := sqlite_migrate.WithInstance(db, &sqlite_migrate.Config{})
	if err != nil {
		return err
	}

	mig, err := migrate.NewWithInstance("iofs", migfs, "sqlite3", migdb)
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
