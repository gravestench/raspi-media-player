package config

import (
	"os"
	"strconv"
)

type Config struct {
	Address      string
	DatabasePath string
	LogFormat    string
	LogLevel     string
	QueueLimit   int
	QueueRate    int
}

func Default() Config {
	return Config{
		Address:      env("RASPI_MEDIA_PLAYER_ADDR", "127.0.0.1:8080"),
		DatabasePath: env("RASPI_MEDIA_PLAYER_DB", "raspi-media-player.sqlite"),
		LogFormat:    env("RASPI_MEDIA_PLAYER_LOG_FORMAT", "text"),
		LogLevel:     env("RASPI_MEDIA_PLAYER_LOG_LEVEL", "info"),
		QueueLimit:   envInt("RASPI_MEDIA_PLAYER_QUEUE_LIMIT", 100),
		QueueRate:    envInt("RASPI_MEDIA_PLAYER_QUEUE_RATE", 20),
	}
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
