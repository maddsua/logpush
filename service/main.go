package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	_ "github.com/lib/pq"
	"github.com/maddsua/logpush/service/dbops"

	"github.com/joho/godotenv"
)

//	todo: add db miration
//	todo: add management API
//	todo: pull txid out of timescale metadata

func main() {

	godotenv.Load()

	if strings.ToLower(os.Getenv("LOG_FMT")) == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	port := os.Getenv("PORT")
	if _, err := strconv.Atoi(port); err != nil {
		port = "8000"
	}

	lokiConn, err := ParseLokiUrl(os.Getenv("LOKI_URL"))
	if err != nil {
		slog.Error("Unable to parse LOKI_HOST",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	if lokiConn != nil {
		slog.Info("Loki connection OK")
	} else {
		slog.Info("Loki not configured. Using Timescale/Postgres")
	}

	dbconn, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("Unable to open DB connection",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	if err := dbconn.Ping(); err != nil {
		slog.Error("Unable to open DB connection",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("DB connection OK")

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

	slog.Info("Starting server",
		slog.String("at", fmt.Sprintf("Listeninig at http://localhost:%s/", port)))

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
		slog.Warn("server shutting down...")
	case err := <-srvSig:
		slog.Error("server crashed",
			slog.String("err", err.Error()))
		os.Exit(1)
	}
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
