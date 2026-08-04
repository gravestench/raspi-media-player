package library

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
)

func TestHouseholdStationsPersonalLibraryAndHistory(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	store := NewStore(db, 90*24*time.Hour)
	stations, err := store.ListStations(ctx, "", "KFJC")
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 1 || stations[0].ID != "household-kfjc" || stations[0].OwnerUserID != "" {
		t.Fatalf("missing household station: %+v", stations)
	}
	if _, err := db.Exec(`INSERT INTO users (id, username, username_key, password_hash) VALUES ('u1', 'One', 'one', 'hash'), ('u2', 'Two', 'two', 'hash')`); err != nil {
		t.Fatal(err)
	}
	personal, err := store.CreateStation(ctx, "u1", "My Radio", "https://example.com/radio")
	if err != nil {
		t.Fatal(err)
	}
	visible, _ := store.ListStations(ctx, "u1", "")
	other, _ := store.ListStations(ctx, "u2", "")
	if len(visible) != 2 || len(other) != 1 {
		t.Fatalf("station visibility: owner=%d other=%d", len(visible), len(other))
	}
	if err := store.SetFavorite(ctx, "u1", personal.ID, true); err != nil {
		t.Fatal(err)
	}
	favorites, _ := store.ListStations(ctx, "u1", "My Radio")
	if len(favorites) != 1 || !favorites[0].Favorite {
		t.Fatalf("favorite missing: %+v", favorites)
	}
	playlist, err := store.CreatePlaylist(ctx, "u1", "Morning")
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.AddPlaylistItem(ctx, "u1", playlist.ID, "KFJC", "direct", stations[0].StreamURL)
	if err != nil {
		t.Fatal(err)
	}
	playlists, _ := store.ListPlaylists(ctx, "u1", "morn")
	if len(playlists) != 1 || len(playlists[0].Items) != 1 {
		t.Fatalf("playlist missing: %+v", playlists)
	}
	if err := store.RemovePlaylistItem(ctx, "u2", playlist.ID, item.ID); err != ErrNotFound {
		t.Fatalf("other user removed item: %v", err)
	}
	if _, err := store.RecordStarted(ctx, "queue-1", "direct", stations[0].StreamURL, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordFinished(ctx, "queue-1", "Test Show", "completed", nil); err != nil {
		t.Fatal(err)
	}
	history, err := store.ListHistory(ctx, "Test Show", 10)
	if err != nil || len(history) != 1 || history[0].Outcome != "completed" {
		t.Fatalf("history: %+v %v", history, err)
	}
}
