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
	"strconv"
	"syscall"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/app"
	"github.com/dylanknuth/raspi-media-player/internal/autoqueue"
	"github.com/dylanknuth/raspi-media-player/internal/config"
	"github.com/dylanknuth/raspi-media-player/internal/database"
	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
	"github.com/dylanknuth/raspi-media-player/internal/library"
	"github.com/dylanknuth/raspi-media-player/internal/logging"
	"github.com/dylanknuth/raspi-media-player/internal/playback"
	"github.com/dylanknuth/raspi-media-player/internal/player"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/settings"
	"github.com/dylanknuth/raspi-media-player/internal/source"
	"github.com/dylanknuth/raspi-media-player/internal/youtube"
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
	flag.BoolVar(&cfg.SetupRequired, "setup-required", cfg.SetupRequired, "require first-run browser installation")
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
	definitions := adminSettingDefinitions(cfg)
	storedSettings, err := settings.NewStore(db, definitions, cfg.SettingsSecretKey)
	if err != nil {
		logger.Error("settings initialization failed", "error", err)
		os.Exit(1)
	}
	applyStoredSettings(context.Background(), storedSettings, &cfg)

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
		imageCache := enrichment.NewImageCache(cfg.MetadataImageDir, nil).WithUserAgent(cfg.MetadataUserAgent)
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
	settingDefinitions := adminSettingDefinitions(cfg)
	youtubeSearcher := youtube.YTDLP{Binary: "yt-dlp", Timeout: 12 * time.Second}
	autoQueue := autoqueue.New(db, queuepkg.NewStore(db), storedSettings, youtubeSearcher, logger, cfg.QueueLimit)
	go autoQueue.Run(ctx)
	handler, err := app.New(logger, db, build, app.Options{QueueLimit: cfg.QueueLimit, QueueRate: cfg.QueueRate, AccessMode: cfg.AccessMode, AuthRate: cfg.AuthRate, SessionLifetime: time.Duration(cfg.SessionDays) * 24 * time.Hour, SecureCookie: cfg.SecureCookie, ArgonMemory: uint32(cfg.ArgonMemory), ArgonIterations: uint32(cfg.ArgonTime), Playback: playbackController, Library: libraryStore, Sources: sourceRegistry, Enrichment: metadataCoordinator, ImageCache: imageCache, SetupRequired: cfg.SetupRequired, Settings: settingDefinitions, SettingsSecretKey: cfg.SettingsSecretKey, YouTubeSearch: youtubeSearcher})
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

func adminSettingDefinitions(cfg config.Config) []settings.Definition {
	return []settings.Definition{
		{Key: "address", Label: "Listen address", Description: "Network address and port; managed in /etc/default.", Category: "Service", Type: "readonly", Value: cfg.Address, ReadOnly: true},
		{Key: "database_path", Label: "Database path", Description: "SQLite data location; managed in /etc/default.", Category: "Service", Type: "readonly", Value: cfg.DatabasePath, ReadOnly: true},
		{Key: "log_format", Label: "Log format", Description: "Structured daemon log format; managed in /etc/default.", Category: "Service", Type: "readonly", Value: cfg.LogFormat, ReadOnly: true},
		{Key: "log_level", Label: "Log level", Description: "Minimum structured log level; managed in /etc/default.", Category: "Service", Type: "readonly", Value: cfg.LogLevel, ReadOnly: true},
		{Key: "access_mode", Label: "Household access", Description: "Choose whether player actions are open or require accounts.", Category: "Access", Type: "select", Value: cfg.AccessMode, Options: []string{"open", "accounts_optional", "accounts_required"}, RestartRequired: true},
		{Key: "auth_rate", Label: "Authentication rate limit", Description: "Login and signup attempts per client each minute.", Category: "Access", Type: "number", Value: fmt.Sprint(cfg.AuthRate), RestartRequired: true},
		{Key: "session_days", Label: "Session lifetime", Description: "Days before local sessions expire.", Category: "Access", Type: "number", Value: fmt.Sprint(cfg.SessionDays), RestartRequired: true},
		{Key: "secure_cookie", Label: "HTTPS-only cookies", Description: "Enable only when this site is served through HTTPS.", Category: "Access", Type: "boolean", Value: fmt.Sprint(cfg.SecureCookie), RestartRequired: true},
		{Key: "argon_memory", Label: "Password hash memory", Description: "Argon2id memory cost in KiB.", Category: "Access", Type: "number", Value: fmt.Sprint(cfg.ArgonMemory), RestartRequired: true},
		{Key: "argon_iterations", Label: "Password hash iterations", Description: "Argon2id iteration count.", Category: "Access", Type: "number", Value: fmt.Sprint(cfg.ArgonTime), RestartRequired: true},
		{Key: "queue_limit", Label: "Queue limit", Description: "Maximum number of queued items.", Category: "Queue", Type: "number", Value: fmt.Sprint(cfg.QueueLimit), RestartRequired: true},
		{Key: "queue_rate", Label: "Queue rate limit", Description: "Anonymous additions per client each minute.", Category: "Queue", Type: "number", Value: fmt.Sprint(cfg.QueueRate), RestartRequired: true},
		{Key: "auto_queue_enabled", Label: "Auto-queue", Description: "Keep the queue filled using the selected recommendation strategy.", Category: "Auto-queue", Type: "boolean", Value: "false"},
		{Key: "auto_queue_mode", Label: "Recommendation strategy", Description: "Rotate fairly through active listeners, use chosen seeds, or follow the last queued item.", Category: "Auto-queue", Type: "select", Value: "active_users", Options: []string{"active_users", "specific_seeds", "related_last"}},
		{Key: "auto_queue_artists", Label: "Seed artists", Description: "Comma-separated artists used by the specific artists or genres strategy.", Category: "Auto-queue", Type: "text", Value: ""},
		{Key: "auto_queue_genres", Label: "Seed genres", Description: "Comma-separated genres used by the specific artists or genres strategy.", Category: "Auto-queue", Type: "text", Value: ""},
		{Key: "auto_queue_depth", Label: "Tracks kept ahead", Description: "Number of automatically selected tracks to keep waiting behind the current track.", Category: "Auto-queue", Type: "number", Value: "3"},
		{Key: "auto_queue_active_seconds", Label: "Active session window", Description: "Seconds since a signed-in browser checked in to influence recommendations.", Category: "Auto-queue", Type: "number", Value: "300"},
		{Key: "audio_device", Label: "Audio device", Description: "mpv/ALSA output device.", Category: "Playback", Type: "text", Value: cfg.AudioDevice, RestartRequired: true},
		{Key: "player_enabled", Label: "Player enabled", Description: "Run the Raspberry Pi audio player.", Category: "Playback", Type: "boolean", Value: fmt.Sprint(cfg.PlayerEnabled), RestartRequired: true},
		{Key: "player_backend", Label: "Player backend", Description: "Audio engine used by the daemon.", Category: "Playback", Type: "select", Value: cfg.PlayerBackend, Options: []string{"mpv", "fake"}, RestartRequired: true},
		{Key: "mpv_binary", Label: "mpv binary", Description: "Absolute path or executable name for mpv.", Category: "Playback", Type: "text", Value: cfg.MPVBinary, RestartRequired: true},
		{Key: "mpv_socket", Label: "mpv socket", Description: "Unix socket used for player control.", Category: "Playback", Type: "text", Value: cfg.MPVSocket, RestartRequired: true},
		{Key: "cache_seconds", Label: "Network cache", Description: "Seconds of network audio to buffer.", Category: "Playback", Type: "number", Value: fmt.Sprint(cfg.CacheSeconds), RestartRequired: true},
		{Key: "player_retries", Label: "Playback retries", Description: "Retries before a failed item is skipped.", Category: "Playback", Type: "number", Value: fmt.Sprint(cfg.PlayerRetries), RestartRequired: true},
		{Key: "history_days", Label: "History retention", Description: "Days to retain listening history; zero disables pruning.", Category: "Library", Type: "number", Value: fmt.Sprint(cfg.HistoryDays), RestartRequired: true},
		{Key: "metadata_enabled", Label: "Artist information", Description: "Use external services for artist context.", Category: "Metadata", Type: "boolean", Value: fmt.Sprint(cfg.MetadataEnabled), RestartRequired: true},
		{Key: "lastfm_api_key", Label: "Last.fm API key", Description: "Optional key for biographies, tags, and similar artists.", Category: "Metadata", Type: "secret", Value: cfg.LastFMAPIKey, Secret: true, RestartRequired: true},
		{Key: "metadata_cache_days", Label: "Metadata cache", Description: "Days to cache artist information.", Category: "Metadata", Type: "number", Value: fmt.Sprint(cfg.MetadataCacheDays), RestartRequired: true},
		{Key: "metadata_user_agent", Label: "Metadata contact", Description: "Descriptive User-Agent required by keyless providers.", Category: "Metadata", Type: "text", Value: cfg.MetadataUserAgent, RestartRequired: true},
		{Key: "metadata_image_dir", Label: "Artist image cache", Description: "Directory for licensed local thumbnails.", Category: "Metadata", Type: "text", Value: cfg.MetadataImageDir, RestartRequired: true},
		{Key: "metadata_max_inflight", Label: "Metadata request budget", Description: "Maximum simultaneous enrichment jobs.", Category: "Metadata", Type: "number", Value: fmt.Sprint(cfg.MetadataMaxInflight), RestartRequired: true},
		{Key: "vote_enabled", Label: "Household voting", Description: "Require household consensus before skipping or removing another listener's queue item.", Category: "Voting", Type: "boolean", Value: "true"},
		{Key: "vote_active_seconds", Label: "Active listener window", Description: "Seconds a browser counts as active.", Category: "Voting", Type: "number", Value: "60"},
		{Key: "vote_timeout_seconds", Label: "Vote timeout", Description: "Seconds before an unused vote expires.", Category: "Voting", Type: "number", Value: "90"},
		{Key: "vote_percent", Label: "Vote threshold", Description: "Percentage of active listeners required.", Category: "Voting", Type: "number", Value: "50"},
		{Key: "youtube_search_enabled", Label: "YouTube search", Description: "Allow discovery searches from the player.", Category: "YouTube", Type: "boolean", Value: "true"},
		{Key: "youtube_search_results", Label: "Search result limit", Description: "Maximum videos returned for each search.", Category: "YouTube", Type: "number", Value: "8"},
	}
}

func applyStoredSettings(ctx context.Context, store *settings.Store, cfg *config.Config) {
	stringSetting := func(key string, destination *string) {
		if value, err := store.Value(ctx, key); err == nil {
			*destination = value
		}
	}
	intSetting := func(key string, destination *int) {
		if value, err := store.Value(ctx, key); err == nil {
			if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed >= 0 {
				*destination = parsed
			}
		}
	}
	boolSetting := func(key string, destination *bool) {
		if value, err := store.Value(ctx, key); err == nil {
			if parsed, parseErr := strconv.ParseBool(value); parseErr == nil {
				*destination = parsed
			}
		}
	}
	stringSetting("access_mode", &cfg.AccessMode)
	stringSetting("audio_device", &cfg.AudioDevice)
	stringSetting("metadata_user_agent", &cfg.MetadataUserAgent)
	stringSetting("lastfm_api_key", &cfg.LastFMAPIKey)
	stringSetting("player_backend", &cfg.PlayerBackend)
	stringSetting("mpv_binary", &cfg.MPVBinary)
	stringSetting("mpv_socket", &cfg.MPVSocket)
	stringSetting("metadata_image_dir", &cfg.MetadataImageDir)
	intSetting("queue_limit", &cfg.QueueLimit)
	intSetting("queue_rate", &cfg.QueueRate)
	intSetting("cache_seconds", &cfg.CacheSeconds)
	intSetting("player_retries", &cfg.PlayerRetries)
	intSetting("history_days", &cfg.HistoryDays)
	intSetting("metadata_cache_days", &cfg.MetadataCacheDays)
	intSetting("metadata_max_inflight", &cfg.MetadataMaxInflight)
	intSetting("auth_rate", &cfg.AuthRate)
	intSetting("session_days", &cfg.SessionDays)
	intSetting("argon_memory", &cfg.ArgonMemory)
	intSetting("argon_iterations", &cfg.ArgonTime)
	boolSetting("metadata_enabled", &cfg.MetadataEnabled)
	boolSetting("secure_cookie", &cfg.SecureCookie)
	boolSetting("player_enabled", &cfg.PlayerEnabled)
}
