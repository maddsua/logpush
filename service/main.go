package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"github.com/maddsua/logpush/service/dbops"

	"github.com/joho/godotenv"
)

//go:embed migrations/*
var migrationsFs embed.FS

//	todo: add management API
//	todo: pull txid out of timescale metadata

func main() {

	godotenv.Load()

	if strings.ToLower(os.Getenv("LOG_FMT")) == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	if strings.ToLower(os.Getenv("DEBUG")) == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Logging enabled")
	}

	port := os.Getenv("PORT")
	if _, err := strconv.Atoi(port); err != nil {
		port = "8000"
	}

	dbconn, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("STARTUP: Unable to open DB connection",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	if err := dbconn.Ping(); err != nil {
		slog.Error("STARTUP: Unable to open DB connection",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("STARTUP: DB connection OK")

	lokiConn, err := ParseLokiUrl(os.Getenv("LOKI_URL"))
	if err != nil {
		slog.Error("STARTUP: Unable to parse LOKI_HOST",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	if lokiConn != nil {
		slog.Info("STARTUP: Loki connection OK")
	} else {
		slog.Info("STARTUP: Loki not configured. Using Timescale/Postgres")
	}

	if strings.ToLower(os.Getenv("DB_MIGRATE")) == "true" {

		slog.Info("STARTUP: Running DB migrations")

		if err := syncDbSchema(dbconn); err != nil {
			slog.Error("STARTUP: DB migration failed",
				slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

	mux := http.NewServeMux()

	ingester := LogIngester{
		Loki:        lokiConn,
		DB:          dbops.New(dbconn),
		Timescale:   &Timescale{DB: dbconn},
		StreamCache: NewStreamCache(),
	}

	mux.Handle("POST /push/stream/{id}", handleMethod(ingester.handleRequest))

	srv := http.Server{
		Handler: mux,
		Addr:    ":" + port,
	}

	slog.Info("STARTUP: Serving http",
		slog.String("at", fmt.Sprintf("http://localhost:%s/", port)))

	exitSig := make(chan os.Signal, 2)
	signal.Notify(exitSig, syscall.SIGINT, syscall.SIGTERM)
	srvSig := make(chan error)

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			srvSig <- err
		}
	}()

	select {
	case <-exitSig:
		srv.Shutdown(context.Background())
		slog.Warn("SERVICE: Server shutting down...")
	case err := <-srvSig:
		slog.Error("SERVICE: server crashed",
			slog.String("err", err.Error()))
		os.Exit(1)
	}
}

func syncDbSchema(dbconn *sql.DB) error {

	migfs, err := iofs.New(migrationsFs, "migrations")
	if err != nil {
		return err
	}

	migdb, err := postgres.WithInstance(dbconn, &postgres.Config{})
	if err != nil {
		return err
	}

	mig, err := migrate.NewWithInstance("iofs", migfs, "postgres", migdb)
	if err != nil {
		return err
	}

	if err := mig.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

func handleMethod(method func(*http.Request) error) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {

		if xff := req.Header.Get("x-forwarded-for"); xff != "" {
			req.RemoteAddr = xff
		} else if host, _, _ := net.SplitHostPort(req.RemoteAddr); host != "" {
			req.RemoteAddr = host
		}

		if err := method(req); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			writer.Write([]byte(err.Error() + "\r\n"))
			return
		}

		writer.Header().Set("content-type", "text/plain")
		writer.WriteHeader(http.StatusNoContent)
	})
}
