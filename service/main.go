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
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/forwarder/loki"
	"github.com/maddsua/logpush/service/forwarder/timescale"
	"github.com/maddsua/logpush/service/ingester"
	rest_rpc "github.com/maddsua/logpush/service/rpc/rest"

	"github.com/joho/godotenv"
)

//go:embed migrations/*
var migrationsFs embed.FS

func main() {

	godotenv.Load()

	if strings.ToLower(os.Getenv("LOG_FMT")) == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	if envBool("DEBUG") {
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
		slog.Error("STARTUP: Unable to connect to the DB",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("STARTUP: DB connection OK")

	lokiConn, err := loki.ParseLokiUrl(os.Getenv("LOKI_URL"), loki.LokiOptions{
		UseStructMeta: envBoolNf("LOKI_STRUCTURED_METADATA"),
		StrictLabels:  envBoolNf("LOKI_STRICT_LABELS"),
	})
	if err != nil {
		slog.Error("STARTUP: Unable to parse LOKI_HOST",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	if lokiConn != nil {

		if err := lokiConn.Ready(); err != nil {
			slog.Error("STARTUP: Loki is not ready",
				slog.String("err", err.Error()))
			os.Exit(1)
		}

		slog.Info("STARTUP: Loki connection OK")

	} else {
		slog.Info("STARTUP: Loki not configured. Using Timescale/Postgres")
	}

	if envBool("DB_MIGRATEDEBUG") {

		slog.Info("STARTUP: Running DB migrations")

		if err := syncDbSchema(dbconn); err != nil {
			slog.Error("STARTUP: DB migration failed",
				slog.String("err", err.Error()))
			os.Exit(1)
		}
	}

	mux := http.NewServeMux()

	ingester := ingester.Ingester{
		Loki:        lokiConn,
		DB:          dbops.New(dbconn),
		Timescale:   &timescale.Timescale{DB: dbconn},
		StreamCache: ingester.NewStreamCache(),
		Opts: ingester.IngesterOptions{
			MaxLabels:       envInt("INGESTER_MAX_LABELS"),
			MaxLabelNameLen: envInt("INGESTER_MAX_LABEL_NAME_LEN"),
			MaxLabelLen:     envInt("INGESTER_MAX_LABEL_LEN"),
			MaxMessages:     envInt("INGESTER_MAX_MESSAGES"),
			MaxMessageLen:   envInt("INGESTER_MAX_MESSAGE_LEN"),
			KeepEmptyLabels: envBoolNf("INGESTER_KEEP_EMPTY_LABELS"),
		},
	}

	mux.HandleFunc("POST /push/stream/{id}", func(writer http.ResponseWriter, req *http.Request) {

		if err := ingester.HandleRequest(req); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			writer.Write([]byte(err.Error() + "\r\n"))
			return
		}

		writer.Header().Set("content-type", "text/plain")
		writer.WriteHeader(http.StatusNoContent)
	})

	if token := strings.TrimSpace(os.Getenv("RPC_TOKEN")); token != "" {
		mux.Handle("/rpc/", http.StripPrefix("/rpc", &rest_rpc.RPCHandler{
			RPCProcedures: rest_rpc.RPCProcedures{
				DB: dbops.New(dbconn),
			},
			Token: token,
			AuthAttempts: rest_rpc.AuthAttempts{
				Attempts: 10,
				Period:   6 * time.Hour,
			},
		}))
	}

	srv := http.Server{
		Handler: rootMiddleware(mux),
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

func rootMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {

		if xff := req.Header.Get("x-forwarded-for"); xff != "" {
			req.RemoteAddr = xff
		} else if host, _, _ := net.SplitHostPort(req.RemoteAddr); host != "" {
			req.RemoteAddr = host
		}

		next.ServeHTTP(writer, req)
	})
}

func envInt(name string) int {

	envVal := os.Getenv(name)
	if envVal == "" {
		return 0
	}

	val, err := strconv.Atoi(envVal)
	if err != nil {
		return 0
	}

	return val
}

func envBool(name string) bool {
	return strings.ToLower(os.Getenv(name)) == "true"
}

func envBoolNf(name string) bool {
	return strings.ToLower(os.Getenv(name)) != "false"
}
