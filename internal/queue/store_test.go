package queue

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/dylanknuth/raspi-media-player/internal/database"
)

func TestPlaybackPreservesCanonicalYouTubeTitleAndUpdatesStreamTitle(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "titles.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	_, video, err := store.AddSourceTitled(context.Background(), "youtube", "https://youtube.test/watch?v=1", "Artist - Canonical Track", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent(context.Background(), video.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlayback(context.Background(), PlaybackState{Status: "playing", Title: "Canonical Track"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(context.Background())
	if err != nil || snapshot.Items[0].Title != "Artist - Canonical Track" {
		t.Fatalf("video title overwritten: %+v err=%v", snapshot.Items, err)
	}
	if _, err := store.Clear(context.Background(), snapshot.Revision); err != nil {
		t.Fatal(err)
	}
	_, stream, err := store.AddSourceTitled(context.Background(), "direct", "https://radio.test/stream", "Artist One - First", "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent(context.Background(), stream.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlayback(context.Background(), PlaybackState{Status: "playing", Title: "Artist Two - Next"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(context.Background())
	if err != nil || snapshot.Items[0].Title != "Artist Two - Next" {
		t.Fatalf("stream title not updated: %+v err=%v", snapshot.Items, err)
	}
}

func TestDefaultRadioStaysAtBackAndSurvivesQueueMutations(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "fallback.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	store := NewStore(db)
	if err := store.EnsureDefault(ctx, "https://radio.test/default", "House Radio"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil || len(snapshot.Items) != 1 || !snapshot.Items[0].Default || snapshot.Items[0].Position != 0 {
		t.Fatalf("initial fallback: %+v err=%v", snapshot, err)
	}
	fallbackID := snapshot.Items[0].ID
	if _, _, err := store.AddSourceTitled(ctx, "youtube", "https://youtube.test/watch?v=request", "Artist - Request", "", nil, 10); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if len(snapshot.Items) != 2 || snapshot.Items[0].Default || !snapshot.Items[1].Default || snapshot.Items[1].ID != fallbackID {
		t.Fatalf("fallback not pinned: %+v", snapshot.Items)
	}
	if _, err := store.Remove(ctx, fallbackID, snapshot.Revision); !errors.Is(err, ErrProtected) {
		t.Fatalf("fallback removal err=%v", err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if _, err := store.Clear(ctx, snapshot.Revision); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if len(snapshot.Items) != 1 || !snapshot.Items[0].Default {
		t.Fatalf("clear removed fallback: %+v", snapshot.Items)
	}
	if err := store.SetCurrent(ctx, fallbackID); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlayback(ctx, PlaybackState{Status: "playing", Title: "Live Artist - Live Track"}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureDefault(ctx, "https://radio.test/default", "House Radio"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if snapshot.Items[0].Title != "Live Artist - Live Track" {
		t.Fatalf("live metadata overwritten: %+v", snapshot.Items[0])
	}
	if err := store.EnsureDefault(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Snapshot(ctx)
	if len(snapshot.Items) != 0 {
		t.Fatalf("fallback not disabled: %+v", snapshot.Items)
	}
}
