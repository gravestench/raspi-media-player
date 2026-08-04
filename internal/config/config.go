package config

import "os"

type Config struct {
	Address      string
	DatabasePath string
	LogFormat    string
	LogLevel     string
}

func Default() Config {
	return Config{
		Address:      env("RASPI_MEDIA_PLAYER_ADDR", "127.0.0.1:8080"),
		DatabasePath: env("RASPI_MEDIA_PLAYER_DB", "raspi-media-player.sqlite"),
		LogFormat:    env("RASPI_MEDIA_PLAYER_LOG_FORMAT", "text"),
		LogLevel:     env("RASPI_MEDIA_PLAYER_LOG_LEVEL", "info"),
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
