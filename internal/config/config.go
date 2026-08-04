package config

import (
	"os"
	"strconv"
)

type Config struct {
	Address       string
	DatabasePath  string
	LogFormat     string
	LogLevel      string
	QueueLimit    int
	QueueRate     int
	AccessMode    string
	AuthRate      int
	SessionDays   int
	SecureCookie  bool
	ArgonMemory   int
	ArgonTime     int
	PlayerEnabled bool
	PlayerBackend string
	MPVBinary     string
	MPVSocket     string
	AudioDevice   string
	CacheSeconds  int
	PlayerRetries int
	HistoryDays   int
	LastFMAPIKey  string
}

func Default() Config {
	return Config{
		Address:       env("RASPI_MEDIA_PLAYER_ADDR", "127.0.0.1:8080"),
		DatabasePath:  env("RASPI_MEDIA_PLAYER_DB", "raspi-media-player.sqlite"),
		LogFormat:     env("RASPI_MEDIA_PLAYER_LOG_FORMAT", "text"),
		LogLevel:      env("RASPI_MEDIA_PLAYER_LOG_LEVEL", "info"),
		QueueLimit:    envInt("RASPI_MEDIA_PLAYER_QUEUE_LIMIT", 100),
		QueueRate:     envInt("RASPI_MEDIA_PLAYER_QUEUE_RATE", 20),
		AccessMode:    env("RASPI_MEDIA_PLAYER_ACCESS_MODE", "open"),
		AuthRate:      envInt("RASPI_MEDIA_PLAYER_AUTH_RATE", 10),
		SessionDays:   envInt("RASPI_MEDIA_PLAYER_SESSION_DAYS", 30),
		SecureCookie:  env("RASPI_MEDIA_PLAYER_SECURE_COOKIE", "false") == "true",
		ArgonMemory:   envInt("RASPI_MEDIA_PLAYER_ARGON_MEMORY_KIB", 65536),
		ArgonTime:     envInt("RASPI_MEDIA_PLAYER_ARGON_ITERATIONS", 3),
		PlayerEnabled: env("RASPI_MEDIA_PLAYER_PLAYER_ENABLED", "false") == "true",
		PlayerBackend: env("RASPI_MEDIA_PLAYER_PLAYER_BACKEND", "mpv"),
		MPVBinary:     env("RASPI_MEDIA_PLAYER_MPV_BINARY", "mpv"),
		MPVSocket:     env("RASPI_MEDIA_PLAYER_MPV_SOCKET", "/var/lib/raspi-media-player/mpv.sock"),
		AudioDevice:   env("RASPI_MEDIA_PLAYER_AUDIO_DEVICE", "auto"),
		CacheSeconds:  envInt("RASPI_MEDIA_PLAYER_CACHE_SECONDS", 20),
		PlayerRetries: envNonnegativeInt("RASPI_MEDIA_PLAYER_PLAYER_RETRIES", 1),
		HistoryDays:   envNonnegativeInt("RASPI_MEDIA_PLAYER_HISTORY_DAYS", 90),
		LastFMAPIKey:  env("RASPI_MEDIA_PLAYER_LASTFM_API_KEY", ""),
	}
}

func envNonnegativeInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err == nil && value >= 0 {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err == nil && value > 0 {
		return value
	}
	return fallback
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
