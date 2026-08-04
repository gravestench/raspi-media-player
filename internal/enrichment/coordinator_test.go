package enrichment

import (
	"context"
	"github.com/dylanknuth/raspi-media-player/internal/database"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type fakeProvider struct{ calls atomic.Int32 }

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Lookup(_ context.Context, h TrackHint) (Result, error) {
	p.calls.Add(1)
	return Result{Hint: h, Provider: p.Name(), Genres: []string{"rock"}}, nil
}
func TestCoordinatorCachesBackgroundLookup(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "c.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p := &fakeProvider{}
	c := NewCoordinator(NewStore(db), time.Hour, p)
	first, err := c.Lookup(context.Background(), "Artist - Song")
	if err != nil || first.Status != "pending" {
		t.Fatalf("first: %+v %v", first, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, err := c.Lookup(context.Background(), "Artist - Song")
		if err == nil && got.Status == "ready" {
			if p.calls.Load() != 1 {
				t.Fatalf("calls=%d", p.calls.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("lookup did not complete")
}
