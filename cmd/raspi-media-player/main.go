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
	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
	"github.com/dylanknuth/raspi-media-player/internal/library"
	"github.com/dylanknuth/raspi-media-player/internal/logging"
	"github.com/dylanknuth/raspi-media-player/internal/playback"
	"github.com/dylanknuth/raspi-media-player/internal/player"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/source"
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
	flag.IntVar(&cfg.QueueLimit, "queue-limit", cfg.QueueLimit, "maximum number of queued items")
	flag.IntVar(&cfg.QueueRate, "queue-rate", cfg.QueueRate, "anonymous queue additions allowed per client per minute")
	flag.StringVar(&cfg.AccessMode, "access-mode", cfg.AccessMode, "access mode: open, accounts_optional, or accounts_required")
	flag.IntVar(&cfg.AuthRate, "auth-rate", cfg.AuthRate, "login/signup attempts allowed per client per minute")
	flag.IntVar(&cfg.SessionDays, "session-days", cfg.SessionDays, "session lifetime in days")
	flag.BoolVar(&cfg.SecureCookie, "secure-cookie", cfg.SecureCookie, "require HTTPS for session cookies")
	flag.IntVar(&cfg.ArgonMemory, "argon-memory", cfg.ArgonMemory, "Argon2id memory in KiB")
	flag.IntVar(&cfg.ArgonTime, "argon-iterations", cfg.ArgonTime, "Argon2id iteration count")
	flag.BoolVar(&cfg.PlayerEnabled, "player-enabled", cfg.PlayerEnabled, "enable mpv playback")
	flag.StringVar(&cfg.PlayerBackend, "player-backend", cfg.PlayerBackend, "player backend: mpv or fake")
	flag.StringVar(&cfg.MPVBinary, "mpv-binary", cfg.MPVBinary, "mpv executable path")
	flag.StringVar(&cfg.MPVSocket, "mpv-socket", cfg.MPVSocket, "mpv IPC Unix socket path")
	flag.StringVar(&cfg.AudioDevice, "audio-device", cfg.AudioDevice, "mpv audio device or auto")
	flag.IntVar(&cfg.CacheSeconds, "cache-seconds", cfg.CacheSeconds, "network media cache duration")
	flag.IntVar(&cfg.PlayerRetries, "player-retries", cfg.PlayerRetries, "media retries before marking an item failed")
	flag.IntVar(&cfg.HistoryDays, "history-days", cfg.HistoryDays, "playback history retention in days; zero disables pruning")
	flag.BoolVar(&cfg.MetadataEnabled, "metadata-enabled", cfg.MetadataEnabled, "enable external artist metadata enrichment")
	flag.IntVar(&cfg.MetadataCacheDays, "metadata-cache-days", cfg.MetadataCacheDays, "artist metadata cache lifetime in days")
	flag.StringVar(&cfg.MetadataUserAgent, "metadata-user-agent", cfg.MetadataUserAgent, "descriptive User-Agent for metadata providers")
	flag.StringVar(&cfg.MetadataImageDir, "metadata-image-dir", cfg.MetadataImageDir, "directory for licensed artist image thumbnails")
	flag.IntVar(&cfg.MetadataMaxInflight, "metadata-max-inflight", cfg.MetadataMaxInflight, "maximum simultaneous artist enrichment jobs")
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var playbackController *playback.Controller
	libraryStore := library.NewStore(db, time.Duration(cfg.HistoryDays)*24*time.Hour)
	sourceRegistry := source.DirectRegistry()
	var metadataCoordinator *enrichment.Coordinator
	if cfg.MetadataEnabled {
		providers := make([]enrichment.Provider, 0, 3)
		if cfg.LastFMAPIKey != "" {
			providers = append(providers, enrichment.NewLastFMProvider(cfg.LastFMAPIKey, nil))
		}
		providers = append(providers, enrichment.NewMusicBrainzProvider(cfg.MetadataUserAgent, nil), enrichment.NewWikimediaProvider(cfg.MetadataUserAgent, nil))
		imageCache := enrichment.NewImageCache(cfg.MetadataImageDir, nil)
		_ = imageCache.PruneOlderThan(time.Duration(cfg.MetadataCacheDays) * 24 * time.Hour)
		metadataCoordinator = enrichment.NewCoordinator(enrichment.NewStore(db), time.Duration(cfg.MetadataCacheDays)*24*time.Hour, providers...).WithImageCache(imageCache).WithMaxInflight(cfg.MetadataMaxInflight)
	}
	if cfg.PlayerEnabled {
		var output player.Player
		switch cfg.PlayerBackend {
		case "mpv":
			output = player.NewMPV(logger, player.MPVConfig{Binary: cfg.MPVBinary, SocketPath: cfg.MPVSocket, AudioDevice: cfg.AudioDevice, CacheSeconds: cfg.CacheSeconds})
		case "fake":
			output = player.NewFake()
		default:
			logger.Error("invalid player backend", "player_backend", cfg.PlayerBackend)
			os.Exit(2)
		}
		playbackController = playback.New(logger, queuepkg.NewStore(db), output, playback.Options{RetryLimit: cfg.PlayerRetries, History: libraryStore, Sources: sourceRegistry, Metadata: metadataCoordinator})
		if err := playbackController.Start(ctx); err != nil {
			logger.Error("playback initialization failed", "error", err)
			os.Exit(1)
		}
		defer func() {
			if err := playbackController.Close(); err != nil {
				logger.Error("playback shutdown failed", "error", err)
			}
		}()
	}

	build := app.BuildInfo{Version: version, Commit: commit, BuiltAt: builtAt}
	var imageCache *enrichment.ImageCache
	if metadataCoordinator != nil {
		imageCache = enrichment.NewImageCache(cfg.MetadataImageDir, nil)
	}
	handler, err := app.New(logger, db, build, app.Options{QueueLimit: cfg.QueueLimit, QueueRate: cfg.QueueRate, AccessMode: cfg.AccessMode, AuthRate: cfg.AuthRate, SessionLifetime: time.Duration(cfg.SessionDays) * 24 * time.Hour, SecureCookie: cfg.SecureCookie, ArgonMemory: uint32(cfg.ArgonMemory), ArgonIterations: uint32(cfg.ArgonTime), Playback: playbackController, Library: libraryStore, Sources: sourceRegistry, Enrichment: metadataCoordinator, ImageCache: imageCache})
	if err != nil {
		logger.Error("application initialization failed", "error", err)
		os.Exit(2)
	}
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

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
