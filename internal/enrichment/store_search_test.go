package enrichment

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
)

func TestStoreSearchFindsGenresTracksAndRelatedArtists(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "discovery.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	value := Result{Hint: TrackHint{Artist: "Alice Coltrane", Title: "Journey in Satchidananda"}, Genres: []string{"spiritual jazz"}, RelatedArtists: []ArtistSummary{{Name: "Pharoah Sanders"}}, Status: "ready", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	if err := store.Put(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"spiritual jazz", "Alice Coltrane", "Journey", "Pharoah Sanders"} {
		matches, err := store.Search(context.Background(), query, 10)
		if err != nil || len(matches) != 1 || matches[0].Hint.Artist != "Alice Coltrane" {
			t.Fatalf("query %q matches=%+v err=%v", query, matches, err)
		}
	}
}
