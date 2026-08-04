package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/app"
	"github.com/dylanknuth/raspi-media-player/internal/config"
	"github.com/dylanknuth/raspi-media-player/internal/database"
	"github.com/dylanknuth/raspi-media-player/internal/logging"
)

var (
	version = "dev"
	commit  = "unknown"
	builtAt = "unknown"
)

func main() {
	cfg := config.Default()
	flag.StringVar(&cfg.Address, "addr", cfg.Address, "HTTP listen address")
	flag.StringVar(&cfg.DatabasePath, "db", cfg.DatabasePath, "SQLite database path")
	flag.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: json or text")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, or error")
	flag.Parse()

	logger, err := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	slog.SetDefault(logger)

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	build := app.BuildInfo{Version: version, Commit: commit, BuiltAt: builtAt}
	handler := app.New(logger, db, build)
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server starting", "address", cfg.Address, "version", version, "database", cfg.DatabasePath)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested", "signal", context.Cause(ctx))
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	logger.Info("server stopped")
}
