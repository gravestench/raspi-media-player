package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

func New(output io.Writer, format, levelName string) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToLower(levelName) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid log level %q", levelName)
	}

	options := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(output, options)
	case "text":
		handler = slog.NewTextHandler(output, options)
	default:
		return nil, fmt.Errorf("invalid log format %q", format)
	}
	return slog.New(handler), nil
}
