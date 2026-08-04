package enrichment

import (
	"context"
	"errors"
	"github.com/dylanknuth/raspi-media-player/internal/database"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTitle(t *testing.T) {
	tests := []struct{ raw, artist, title string }{{"Niki & The Dove - Play It On My Radio", "Niki & The Dove", "Play It On My Radio"}, {"Björk – Jóga (Official Music Video)", "Björk", "Jóga"}, {"Roy Rogers — Shake Your Moneymaker [Official Audio]", "Roy Rogers", "Shake Your Moneymaker"}, {"Unknown free-form title", "", "Unknown free-form title"}}
	for _, test := range tests {
		hint := ParseTitle(test.raw)
		if hint.Artist != test.artist || hint.Title != test.title {
			t.Errorf("ParseTitle(%q) = %+v", test.raw, hint)
		}
	}
}

func TestPersistentEnrichmentCache(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "enrichment.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	hint := ParseTitle("Niki & The Dove - Play It On My Radio")
	if _, err := store.Get(context.Background(), Key(hint)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty cache: %v", err)
	}
	value := Result{Hint: hint, Provider: "test", ArtistURL: "https://example.com/artist", Image: Image{URL: "https://example.com/image.jpg", SourceURL: "https://example.com/license", Attribution: "Example / CC BY"}, Biography: "Biography", Genres: []string{"electropop"}, RelatedArtists: []ArtistSummary{{Name: "Related"}}, Status: "ready", FetchedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)}
	if err := store.Put(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), Key(hint))
	if err != nil {
		t.Fatal(err)
	}
	if got.Hint.Artist != hint.Artist || got.Image.Attribution == "" || len(got.Genres) != 1 || len(got.RelatedArtists) != 1 {
		t.Fatalf("cache result: %+v", got)
	}
}
