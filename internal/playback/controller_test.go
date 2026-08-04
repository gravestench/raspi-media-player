package playback

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
	"github.com/dylanknuth/raspi-media-player/internal/player"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
)

func playbackFixture(t *testing.T, options ...Options) (*queuepkg.Store, *player.Fake, *Controller) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "playback.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := queuepkg.NewStore(db)
	fake := player.NewFake()
	controller := New(slog.New(slog.NewTextHandler(io.Discard, nil)), store, fake, options...)
	if err := controller.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { controller.Close() })
	return store, fake, controller
}

func TestControllerRetriesBeforeFailure(t *testing.T) {
	store, fake, _ := playbackFixture(t, Options{RetryLimit: 1})
	ctx := context.Background()
	_, first, err := store.Add(ctx, "https://example.com/retry.mp3", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := store.Add(ctx, "https://example.com/after-retry.mp3", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "initial retry item load", func() bool { return len(fake.URLs()) == 1 })
	fake.Emit(player.Event{Type: player.EventFailed, Error: errors.New("temporary failure")})
	waitFor(t, "retry item reload", func() bool { urls := fake.URLs(); return len(urls) >= 2 && urls[1] == first.Source.URL })
	fake.Emit(player.Event{Type: player.EventFailed, Error: errors.New("permanent failure")})
	waitFor(t, "advance after retry exhaustion", func() bool { urls := fake.URLs(); return len(urls) >= 3 && urls[2] == second.Source.URL })
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Items[0].ID != first.ID || snapshot.Items[0].Status != "failed" || snapshot.Items[0].Error != "permanent failure" {
		t.Fatalf("retry exhaustion not retained: %+v", snapshot.Items)
	}
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func TestControllerLoadsAdvancesAndKeepsFailedItems(t *testing.T) {
	store, fake, _ := playbackFixture(t)
	ctx := context.Background()
	_, first, err := store.Add(ctx, "https://example.com/first.mp3", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := store.Add(ctx, "https://example.com/second.mp3", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "first item load", func() bool { urls := fake.URLs(); return len(urls) == 1 && urls[0] == first.Source.URL })
	fake.Emit(player.Event{Type: player.EventFailed, Error: errors.New("decoder failed")})
	waitFor(t, "second item load", func() bool { urls := fake.URLs(); return len(urls) >= 2 && urls[1] == second.Source.URL })
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Items[0].ID != first.ID || snapshot.Items[0].Status != "failed" || snapshot.Items[0].Error != "decoder failed" {
		t.Fatalf("failed item not retained: %+v", snapshot.Items)
	}
	fake.Emit(player.Event{Type: player.EventEnded})
	waitFor(t, "completed item removal", func() bool {
		value, _ := store.Snapshot(ctx)
		return len(value.Items) == 1 && value.Items[0].ID == first.ID
	})
	fake.Emit(player.Event{Type: player.EventState, State: player.State{Status: "idle", Title: "stale title", DurationSeconds: 12, Volume: 100}})
	waitFor(t, "idle metadata reset", func() bool {
		value, _ := store.Snapshot(ctx)
		return value.Playback.Status == "idle" && value.Playback.Title == "" && value.Playback.DurationSeconds == 0
	})
}

func TestUnsupportedProviderDoesNotBlockDirectPlayback(t *testing.T) {
	store, fake, _ := playbackFixture(t)
	ctx := context.Background()
	_, unsupported, err := store.AddSource(ctx, "provider-outage", "provider:item", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, direct, err := store.Add(ctx, "https://example.com/still-works.mp3", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "direct playback after unsupported provider", func() bool {
		urls := fake.URLs()
		return len(urls) == 1 && urls[0] == direct.Source.URL
	})
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Items[0].ID != unsupported.ID || snapshot.Items[0].Status != "failed" {
		t.Fatalf("unsupported provider failure not retained: %+v", snapshot.Items)
	}
}

func TestControllerControlsAndStopResume(t *testing.T) {
	store, fake, controller := playbackFixture(t)
	ctx := context.Background()
	_, _, err := store.Add(ctx, "https://example.com/control.mp3", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "item load", func() bool { return len(fake.URLs()) == 1 })
	if err := controller.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if err := controller.Seek(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if err := controller.SetVolume(ctx, 37); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "control state", func() bool {
		snapshot, _ := store.Snapshot(ctx)
		return snapshot.Playback.Paused && snapshot.Playback.PositionSeconds == 42 && snapshot.Playback.Volume == 37
	})
	fake.Emit(player.Event{Type: player.EventState, State: player.State{Status: "unavailable", Error: "mpv crashed"}})
	waitFor(t, "restart volume restore", func() bool {
		snapshot, _ := store.Snapshot(ctx)
		return len(fake.URLs()) >= 2 && fake.Snapshot().Volume == 37 && snapshot.Playback.Volume == 37
	})
	loadedBeforeStop := len(fake.URLs())
	if err := controller.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	if len(fake.URLs()) != loadedBeforeStop {
		t.Fatalf("stopped item reloaded: %v", fake.URLs())
	}
	if err := controller.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "resume reload", func() bool { return len(fake.URLs()) == loadedBeforeStop+1 })
}
