package fallback

import (
	"context"
	"log/slog"
	"strings"
	"time"

	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/settings"
)

type Manager struct {
	queue    *queuepkg.Store
	settings *settings.Store
	logger   *slog.Logger
}

func New(queue *queuepkg.Store, settings *settings.Store, logger *slog.Logger) *Manager {
	return &Manager{queue: queue, settings: settings, logger: logger}
}

func (m *Manager) Sync(ctx context.Context) error {
	streamURL, err := m.settings.Value(ctx, "default_radio_url")
	if err != nil {
		return err
	}
	name, err := m.settings.Value(ctx, "default_radio_name")
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = "Default radio"
	}
	return m.queue.EnsureDefault(ctx, streamURL, name)
}

func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.Sync(ctx); err != nil {
				m.logger.WarnContext(ctx, "default radio synchronization failed", "error", err)
			}
		}
	}
}
