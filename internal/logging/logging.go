package logging

import (
	"context"
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
	return slog.New(&redactingHandler{next: handler}), nil
}

type redactingHandler struct {
	next slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clone := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		clone.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, clone)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, len(attrs))
	for i, attr := range attrs {
		redacted[i] = redactAttr(attr)
	}
	return &redactingHandler{next: h.next.WithAttrs(redacted)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if attr.Value.Kind() == slog.KindGroup {
		members := attr.Value.Group()
		redacted := make([]any, 0, len(members))
		for _, member := range members {
			redacted = append(redacted, redactAttr(member))
		}
		return slog.Group(attr.Key, redacted...)
	}
	key := strings.ToLower(attr.Key)
	for _, sensitive := range []string{"password", "passwd", "authorization", "cookie", "session", "token", "secret", "credential"} {
		if strings.Contains(key, sensitive) {
			return slog.String(attr.Key, "[REDACTED]")
		}
	}
	return attr
}
