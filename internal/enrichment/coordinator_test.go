package enrichment

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
)

type fakeProvider struct{ calls atomic.Int32 }

func (p *fakeProvider) Name() string { return "fake" }
func (p *fakeProvider) Lookup(_ context.Context, h TrackHint) (Result, error) {
	p.calls.Add(1)
	return Result{Hint: h, Provider: p.Name(), Genres: []string{"rock"}}, nil
}

type failingProvider struct{ calls atomic.Int32 }

func (p *failingProvider) Name() string { return "down" }
func (p *failingProvider) Lookup(context.Context, TrackHint) (Result, error) {
	p.calls.Add(1)
	return Result{}, errors.New("provider down")
}
func TestCoordinatorNegativelyCachesProviderOutage(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "negative.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider := &failingProvider{}
	coordinator := NewCoordinator(NewStore(db), time.Hour, provider)
	_, _ = coordinator.Lookup(context.Background(), "Artist - Song")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, err := coordinator.Lookup(context.Background(), "Artist - Song")
		if err == nil && got.Status == "not_found" {
			_, _ = coordinator.Lookup(context.Background(), "Artist - Song")
			if provider.calls.Load() != 1 {
				t.Fatalf("calls=%d", provider.calls.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("negative cache not written")
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
