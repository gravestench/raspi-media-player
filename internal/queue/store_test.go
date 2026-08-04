package queue

import (
	"context"
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
