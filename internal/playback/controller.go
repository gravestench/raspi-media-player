package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/player"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/source"
)

type Controller struct {
	logger        *slog.Logger
	queue         *queuepkg.Store
	player        player.Player
	mu            sync.Mutex
	loadedID      string
	stopped       bool
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	retryLimit    int
	retries       map[string]int
	retryAfter    time.Time
	history       HistoryRecorder
	desiredVolume int
	sources       source.Resolver
	metadata      MetadataObserver
}

type HistoryRecorder interface {
	RecordStarted(context.Context, string, string, string, string) (string, error)
	RecordFinished(context.Context, string, string, string, error) error
}
type MetadataObserver interface{ ObserveTitle(context.Context, string) }

type Options struct {
	RetryLimit int
	History    HistoryRecorder
	Sources    source.Resolver
	Metadata   MetadataObserver
}

func New(logger *slog.Logger, queue *queuepkg.Store, output player.Player, options ...Options) *Controller {
	retryLimit := 0
	if len(options) > 0 && options[0].RetryLimit > 0 {
		retryLimit = options[0].RetryLimit
	}
	var history HistoryRecorder
	if len(options) > 0 {
		history = options[0].History
	}
	var sources source.Resolver = source.DirectRegistry()
	if len(options) > 0 && options[0].Sources != nil {
		sources = options[0].Sources
	}
	var metadata MetadataObserver
	if len(options) > 0 {
		metadata = options[0].Metadata
	}
	return &Controller{logger: logger, queue: queue, player: output, done: make(chan struct{}), retryLimit: retryLimit, retries: make(map[string]int), history: history, sources: sources, metadata: metadata}
}

func (c *Controller) Start(parent context.Context) error {
	c.ctx, c.cancel = context.WithCancel(parent)
	if err := c.queue.ReconcilePlayback(c.ctx); err != nil {
		return fmt.Errorf("reconcile playback: %w", err)
	}
	if err := c.player.Start(c.ctx); err != nil {
		return fmt.Errorf("start player: %w", err)
	}
	go c.run()
	return nil
}

func (c *Controller) run() {
	defer close(c.done)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case event := <-c.player.Events():
			c.handleEvent(event)
		case <-ticker.C:
			c.reconcile()
		}
	}
}

func (c *Controller) reconcile() {
	c.mu.Lock()
	retryAfter := c.retryAfter
	c.mu.Unlock()
	if !retryAfter.IsZero() && time.Now().Before(retryAfter) {
		return
	}
	snapshot, err := c.queue.Snapshot(c.ctx)
	if err != nil {
		c.logger.Error("playback queue reconciliation failed", "error", err)
		return
	}
	c.mu.Lock()
	loadedID, stopped := c.loadedID, c.stopped
	c.desiredVolume = snapshot.Playback.Volume
	c.mu.Unlock()
	if stopped {
		return
	}
	var next *queuepkg.Item
	for i := range snapshot.Items {
		if snapshot.Items[i].Status == "queued" || snapshot.Items[i].Status == "current" {
			next = &snapshot.Items[i]
			break
		}
	}
	if next == nil {
		if loadedID != "" {
			_ = c.player.Stop(c.ctx)
			c.mu.Lock()
			c.loadedID = ""
			c.mu.Unlock()
		}
		if snapshot.Playback.Status != "idle" && snapshot.Playback.Status != "stopped" {
			_ = c.queue.ResetPlayback(c.ctx, "idle", "")
		}
		return
	}
	if next.ID == loadedID {
		return
	}
	if err := c.queue.SetCurrent(c.ctx, next.ID); err != nil {
		if !errors.Is(err, queuepkg.ErrNotFound) {
			c.logger.Error("set current queue item failed", "error", err, "queue_item_id", next.ID)
		}
		return
	}
	playable, err := c.sources.Resolve(c.ctx, next.Source.Kind, next.Source.URL)
	if err != nil {
		_ = c.queue.FinishCurrent(c.ctx, next.ID, fmt.Errorf("resolve source: %w", err))
		c.logger.Warn("source resolution failed", "error", err, "queue_item_id", next.ID, "source_kind", next.Source.Kind)
		return
	}
	if err := c.player.Load(c.ctx, playable.PlaybackURL); err != nil {
		if errors.Is(err, player.ErrUnavailable) {
			_ = c.queue.ResetPlayback(c.ctx, "unavailable", err.Error())
			return
		}
		_ = c.queue.FinishCurrent(c.ctx, next.ID, err)
		c.logger.Error("load media failed", "error", err, "queue_item_id", next.ID)
		return
	}
	if err := c.player.SetVolume(c.ctx, snapshot.Playback.Volume); err != nil {
		c.logger.Warn("restore playback volume failed", "error", err, "volume", snapshot.Playback.Volume)
	}
	c.mu.Lock()
	c.loadedID = next.ID
	c.mu.Unlock()
	if c.history != nil {
		if _, err := c.history.RecordStarted(c.ctx, next.ID, next.Source.Kind, next.Source.URL, next.Submitter.UserID); err != nil {
			c.logger.Error("record playback history start failed", "error", err)
		}
	}
	c.logger.Info("playback item loaded", "queue_item_id", next.ID, "source_kind", next.Source.Kind)
}

func (c *Controller) handleEvent(event player.Event) {
	c.mu.Lock()
	loadedID := c.loadedID
	c.mu.Unlock()
	switch event.Type {
	case player.EventState:
		c.mu.Lock()
		stopped := c.stopped
		c.mu.Unlock()
		if stopped {
			return
		}
		if event.State.Status == "unavailable" {
			if err := c.queue.ResetPlayback(c.ctx, "unavailable", event.State.Error); err != nil {
				c.logger.Error("persist unavailable player state failed", "error", err)
			}
			c.mu.Lock()
			c.loadedID = ""
			c.mu.Unlock()
			return
		}
		if loadedID == "" && event.State.Status == "idle" {
			if err := c.queue.ResetPlayback(c.ctx, "idle", ""); err != nil {
				c.logger.Error("clear idle playback state failed", "error", err)
			}
			return
		}
		if loadedID == "" {
			return
		}
		c.mu.Lock()
		desiredVolume := c.desiredVolume
		c.mu.Unlock()
		state := queuepkg.PlaybackState{Status: event.State.Status, Title: event.State.Title, PositionSeconds: event.State.PositionSeconds, DurationSeconds: event.State.DurationSeconds, Paused: event.State.Paused, Buffering: event.State.Buffering, Volume: desiredVolume, Error: event.State.Error}
		if err := c.queue.UpdatePlayback(c.ctx, state); err != nil {
			c.logger.Error("persist playback state failed", "error", err)
		}
		if c.metadata != nil && state.Title != "" {
			c.metadata.ObserveTitle(c.ctx, state.Title)
		}
	case player.EventEnded:
		if loadedID != "" {
			c.mu.Lock()
			delete(c.retries, loadedID)
			c.mu.Unlock()
			if c.history != nil {
				if err := c.history.RecordFinished(c.ctx, loadedID, event.State.Title, "completed", nil); err != nil {
					c.logger.Error("record playback completion failed", "error", err)
				}
			}
			if err := c.queue.FinishCurrent(c.ctx, loadedID, nil); err != nil && !errors.Is(err, queuepkg.ErrNotFound) {
				c.logger.Error("advance completed item failed", "error", err)
			}
			c.mu.Lock()
			c.loadedID = ""
			c.mu.Unlock()
		}
	case player.EventFailed:
		failure := event.Error
		if failure == nil {
			failure = errors.New("media playback failed")
		}
		if loadedID != "" {
			c.mu.Lock()
			attempts := c.retries[loadedID]
			if attempts < c.retryLimit {
				c.retries[loadedID] = attempts + 1
				c.loadedID = ""
				c.retryAfter = time.Now().Add(500 * time.Millisecond)
				c.mu.Unlock()
				if err := c.queue.RetryCurrent(c.ctx, loadedID, failure); err != nil {
					c.logger.Error("schedule media retry failed", "error", err)
				}
				c.logger.Warn("retrying failed media item", "error", failure, "queue_item_id", loadedID, "attempt", attempts+1, "retry_limit", c.retryLimit)
				return
			}
			delete(c.retries, loadedID)
			c.mu.Unlock()
			if c.history != nil {
				if err := c.history.RecordFinished(c.ctx, loadedID, event.State.Title, "failed", failure); err != nil {
					c.logger.Error("record playback failure failed", "error", err)
				}
			}
			if err := c.queue.FinishCurrent(c.ctx, loadedID, failure); err != nil && !errors.Is(err, queuepkg.ErrNotFound) {
				c.logger.Error("mark failed item failed", "error", err)
			}
			c.logger.Warn("media item failed", "error", failure, "queue_item_id", loadedID)
			c.mu.Lock()
			c.loadedID = ""
			c.mu.Unlock()
		}
	}
}

func (c *Controller) Pause(ctx context.Context) error { return c.player.SetPaused(ctx, true) }
func (c *Controller) Resume(ctx context.Context) error {
	snapshot, err := c.queue.Snapshot(ctx)
	if err != nil {
		return err
	}
	if snapshot.Playback.Status == "stopped" {
		c.mu.Lock()
		c.loadedID = ""
		c.stopped = false
		c.mu.Unlock()
		if err := c.queue.ReconcilePlayback(ctx); err != nil {
			return err
		}
		return nil
	}
	return c.player.SetPaused(ctx, false)
}
func (c *Controller) Stop(ctx context.Context) error {
	if err := c.player.Stop(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.loadedID = ""
	c.stopped = true
	c.mu.Unlock()
	return c.queue.ResetPlayback(ctx, "stopped", "")
}
func (c *Controller) Seek(ctx context.Context, seconds float64) error {
	return c.player.Seek(ctx, seconds)
}
func (c *Controller) SetVolume(ctx context.Context, volume int) error {
	c.mu.Lock()
	previous := c.desiredVolume
	c.desiredVolume = volume
	c.mu.Unlock()
	if err := c.player.SetVolume(ctx, volume); err != nil {
		c.mu.Lock()
		c.desiredVolume = previous
		c.mu.Unlock()
		return err
	}
	return c.queue.SetVolume(ctx, volume)
}
func (c *Controller) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	_ = c.player.Close()
	select {
	case <-c.done:
	case <-time.After(5 * time.Second):
		return errors.New("playback controller shutdown timed out")
	}
	return nil
}
