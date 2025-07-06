package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/maddsua/logpush"
)

type CliFlags struct {
	Cfg      *string
	Debug    *bool
	JsonLogs *bool
}

func main() {

	godotenv.Load()

	cli := CliFlags{
		Cfg:      flag.String("cfg", "", "config file location"),
		Debug:    flag.Bool("debug", false, "enable debug logging"),
		JsonLogs: flag.Bool("json_logs", false, "log in json format"),
	}
	flag.Parse()

	if os.Getenv("DEBUG") == "true" || *cli.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Enabled")
	}

	if os.Getenv("LOGFMT") == "json" || *cli.JsonLogs {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	}

	if *cli.Cfg == "" {
		if loc, has := FindConfig([]string{
			"./logpush.yml",
			"/etc/mws/logpush/logpush.yml",
		}); has {
			cli.Cfg = &loc
		}
	}

	if *cli.Cfg == "" {
		slog.Error("No config files found")
		os.Exit(1)
	}

	cfg, err := LoadConfigFile(*cli.Cfg)
	if err != nil {
		slog.Error("Failed to load config",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("Config location",
		slog.String("file", *cli.Cfg))

	if len(cfg.Streams) > 0 {
		for key, val := range cfg.Streams {
			slog.Info("Add stream",
				slog.String("key", key),
				slog.String("tag", val.Tag),
				slog.Bool("with_token", val.Token != ""))
		}
	} else {
		slog.Warn("No streams found in config")
	}

	var writer logpush.LogWriter
	if val := os.Getenv("TIMESCALE_URL"); val != "" {

		timescale, err := logpush.NewTimescaleWriter(val)
		if err != nil {
			fmt.Println("logpush.NewLokiWriter", err)
			os.Exit(1)
		}

		defer timescale.Close()

		slog.Info("USING TIMESCALE WRITER")

		writer = timescale

	} else if val := os.Getenv("LOKI_URL"); val != "" {

		loki, err := logpush.NewLokiWriter(val)
		if err != nil {
			fmt.Println("logpush.NewLokiWriter", err)
			os.Exit(1)
		}

		slog.Info("USING LOKI WRITER",
			slog.Bool("structured_meta", loki.UseStructMeta))

		writer = loki
	} else {
		slog.Warn("USING STDOUT WRITER")
		writer = &StdoutWriter{}
	}

	var mux http.ServeMux

	mux.Handle("POST /push/stream/{stream_key}", &logpush.LogIngester{
		Writer:  writer,
		Streams: cfg.Streams,
		Options: cfg.Ingester,
	})

	mux.HandleFunc("/health", func(wrt http.ResponseWriter, _ *http.Request) {
		wrt.WriteHeader(http.StatusNoContent)
	})

	port := os.Getenv("PORT")
	if _, err := strconv.Atoi(port); err != nil || port == "" {
		port = "13666"
	}

	srv := http.Server{
		Addr:    ":" + port,
		Handler: &mux,
	}

	errorCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			errorCh <- err
		}
	}()

	slog.Info("Starting server",
		slog.String("at", fmt.Sprintf("http://localhost%s", srv.Addr)))

	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-exitCh:
		slog.Warn("Shutting down...")
		srv.Shutdown(context.Background())
	case err := <-errorCh:
		slog.Error("Shutting down...",
			slog.String("err", err.Error()))
		os.Exit(1)
	}
}
