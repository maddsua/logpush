package timescale

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"

	_ "github.com/lib/pq"
	"github.com/maddsua/logpush/service/logs"
	"github.com/maddsua/logpush/service/timescale/queries"

	"github.com/golang-migrate/migrate/v4"
	postgres_migrate "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*
var migfs embed.FS

func NewCollector(dbUrl string) (*timescale, error) {

	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		return nil, err
	}

	this := &timescale{db: db, queries: queries.New(db)}

	if err := this.migrate(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %s", err.Error())
	}

	return this, nil
}

type timescale struct {
	db      *sql.DB
	queries *queries.Queries
}

func (this *timescale) Close() error {
	return this.db.Close()
}

func (this *timescale) Push(entries []logs.Entry) error {

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

func (this *timescale) migrate(db *sql.DB) error {

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

	slog.Debug("Timescale collector: DB migrated",
		slog.Int("version", int(version)),
		slog.Bool("dirty", ditry))

	return nil
}
