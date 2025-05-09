package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/joho/godotenv"
	"github.com/maddsua/logpush/service/logs"
	"github.com/maddsua/logpush/service/loki"
	"github.com/maddsua/logpush/service/timescale"
)

//	todo: exporter api

func main() {

	godotenv.Load()

	flagDebug := flag.Bool("debug", false, "Show debug logging")
	flagConfigFile := flag.String("config", "./logpush.config.yml", "Set config value path")
	flagLogFmt := flag.String("logfmt", "", "Log format: json|null")
	flag.Parse()

	if strings.ToLower(os.Getenv("LOG_FMT")) == "json" || strings.ToLower(*flagLogFmt) == "json" {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	}

	slog.Info("STARTUP: Starting logpush service")

	if *flagDebug || strings.ToLower(os.Getenv("LOG_LEVEL")) == "debug" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Enabled")
	}

	slog.Info("STARTUP: Config file located",
		slog.String("at", *flagConfigFile))

	cfg, err := loadConfigFile(*flagConfigFile)
	if err != nil {
		slog.Error("STARTUP: Failed to load config file",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	if err := cfg.Valid(); err != nil {
		slog.Error("STARTUP: Failed to validate config file",
			slog.String("err", err.Error()))
		os.Exit(1)
	}

	var collector logs.Collector

	if val := os.Getenv("DATABASE_URL"); val != "" {

		var dbUrl url.URL
		if parsed, err := url.Parse(val); err == nil {
			dbUrl.Scheme = parsed.Scheme
			dbUrl.Host = parsed.Host
		}

		slog.Info("STARTUP: Using Timescale data collector",
			slog.String("url", dbUrl.String()))

		driver, err := timescale.NewCollector(val)
		if err != nil {
			slog.Error("STARTUP: Failed to initialize timescale collector",
				slog.String("err", err.Error()))
			os.Exit(1)
		}
		collector = driver

	} else if val := os.Getenv("LOKI_URL"); val != "" {

		slog.Info("STARTUP: Using Loki data collector",
			slog.String("url", val))

		driver, err := loki.NewCollector(val)
		if err != nil {
			slog.Error("STARTUP: Failed to initialize loki collector",
				slog.String("err", err.Error()))
			os.Exit(1)
		}
		collector = driver
	}

	if collector == nil {
		slog.Error("STARTUP: No data collectors configured")
		os.Exit(1)
	}

	defer collector.Close()

	ingester := LogIngester{
		Collector: collector,
		Cfg:       cfg.Ingester,
		Streams:   cfg.Streams,
	}

	mux := http.NewServeMux()

	mux.Handle("POST /push/stream/{id}", &ingester)

	port := os.Getenv("PORT")
	if _, err := strconv.Atoi(port); err != nil || port == "" {
		port = "7300"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	slog.Info("STARTUP: Starting HTTP server now",
		slog.String("addr", srv.Addr))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := srv.ListenAndServe(); err != nil && ctx.Err() == nil {
			slog.Error("SERVICE: API server error",
				slog.String("err", err.Error()))
			os.Exit(1)
		}
	}()

	defer func() {
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("SERVICE: Error shutting server down",
				slog.String("err", err.Error()))
		}
	}()

	exit := make(chan os.Signal, 2)
	signal.Notify(exit, syscall.SIGINT, syscall.SIGTERM)

	<-exit

	slog.Info("SERVICE: Shutting down...")
	cancel()
}

func loadConfigFile(path string) (*RootConfig, error) {

	file, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %s", err.Error())
	}

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get config file info: %s", err.Error())
	}

	if !info.Mode().IsRegular() {
		return nil, errors.New("failed to read config file: config file must be a regular file")
	}

	var cfg RootConfig

	if strings.HasSuffix(path, ".yml") {
		if err := yaml.NewDecoder(file).Decode(&cfg); err != nil {
			return nil, fmt.Errorf("failed to decode config file: %s", err.Error())
		}
	} else if strings.HasSuffix(path, ".json") {
		if err := json.NewDecoder(file).Decode(&cfg); err != nil {
			return nil, fmt.Errorf("failed to decode config file: %s", err.Error())
		}
	} else {
		return nil, errors.New("unsupported config file format")
	}

	return &cfg, nil
}
