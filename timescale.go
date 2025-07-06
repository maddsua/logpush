package logpush

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type sqltx interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewTimescaleWriter(dbUrl string) (*timescaleWriter, error) {

	const version = "v2"

	db, err := sql.Open("postgres", dbUrl)
	if err != nil {
		return nil, err
	}

	tableName := "logpush_entries_" + version

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var tableExists = func() (bool, error) {

		query := fmt.Sprintf("select exists (select 1 from %s)", tableName)

		_, err := db.QueryContext(ctx, query)
		if err == nil || strings.Contains(err.Error(), "does not exist") {
			return err == nil, nil
		}

		return false, err
	}

	var tableInit = func() error {

		query := fmt.Sprintf(`create table %s (
			time timestamp with time zone not null,
			tag text not null,
			level text not null,
			message text not null,
			meta jsonb null
		)`, tableName)

		_, err := db.ExecContext(ctx, query)
		return err
	}

	if exists, _ := tableExists(); !exists {

		slog.Info("TIMESCALE: Setting up",
			slog.String("table", tableName))

		if err := tableInit(); err != nil {
			db.Close()
			return nil, err
		}
	}

	return &timescaleWriter{
		db:      db,
		table:   tableName,
		version: version,
	}, nil
}

type timescaleWriter struct {
	db      *sql.DB
	table   string
	version string
}

func (this *timescaleWriter) Type() string {
	return "timescale"
}

func (this *timescaleWriter) Version() string {
	return this.version
}

func (this *timescaleWriter) Ping() error {
	return this.db.Ping()
}

func (this *timescaleWriter) Close() error {
	return this.db.Close()
}

func (this *timescaleWriter) WriteEntry(ctx context.Context, entry LogEntry) error {
	return this.writeEntryTx(ctx, this.db, entry)
}

func (this *timescaleWriter) WriteBatch(ctx context.Context, batch []LogEntry) error {

	tx, err := this.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, val := range batch {
		if err := this.writeEntryTx(ctx, tx, val); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (this *timescaleWriter) writeEntryTx(ctx context.Context, tx sqltx, entry LogEntry) error {

	row := map[string]any{
		"time":    entry.Timestamp,
		"tag":     entry.StreamTag,
		"level":   entry.LogLevel.String(),
		"message": entry.Message,
		"meta":    entry.Metadata,
	}

	if entry.Metadata != nil {
		data, err := json.Marshal(entry.Metadata)
		if err != nil {
			return err
		}
		row["meta"] = string(data)
	}

	return sqlInsertContext(ctx, tx, this.table, row)
}

func sqlInsertContext(ctx context.Context, tx sqltx, table string, row map[string]any) error {

	var columns []string
	var args []any
	for col, val := range row {
		columns = append(columns, col)
		args = append(args, val)
	}

	var bindvars []string
	for idx := range columns {
		bindvars = append(bindvars, "$"+strconv.Itoa(idx+1))
	}

	query := fmt.Sprintf("insert into %s (%s) values (%s)",
		table,
		strings.Join(columns, ", "),
		strings.Join(bindvars, ", "))

	_, err := tx.ExecContext(ctx, query, args...)
	return err
}
