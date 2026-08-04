package source

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirectAdapterContract(t *testing.T) {
	registry := DirectRegistry()
	kind, err := registry.Classify("https://example.com/radio.mp3")
	if err != nil || kind != "direct" {
		t.Fatalf("classify direct: %q %v", kind, err)
	}
	playable, err := registry.Resolve(context.Background(), kind, "https://example.com/radio.mp3")
	if err != nil || playable.PlaybackURL != playable.OriginalURL {
		t.Fatalf("resolve direct: %+v %v", playable, err)
	}
	for _, invalid := range []string{"", "file:///etc/passwd", "https://user:secret@example.com/audio", "not a url"} {
		if _, err := registry.Classify(invalid); !errors.Is(err, ErrInvalid) {
			t.Errorf("classify %q: %v", invalid, err)
		}
	}
	if _, err := registry.Classify("https://www.youtube.com/watch?v=example"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("classify disabled provider: %v", err)
	}
}

type countingAdapter struct{ calls atomic.Int32 }

func (a *countingAdapter) Kind() string            { return "test" }
func (a *countingAdapter) Match(value string) bool { return value == "test:value" }
func (a *countingAdapter) Resolve(_ context.Context, value string) (Playable, error) {
	a.calls.Add(1)
	return Playable{Kind: a.Kind(), OriginalURL: value, PlaybackURL: "https://example.com/resolved"}, nil
}

func TestRegistryCachesAdapterResolution(t *testing.T) {
	adapter := &countingAdapter{}
	registry := NewRegistry(time.Second, time.Minute, adapter)
	for range 2 {
		if _, err := registry.Resolve(context.Background(), "test", "test:value"); err != nil {
			t.Fatal(err)
		}
	}
	if adapter.calls.Load() != 1 {
		t.Fatalf("resolve calls = %d", adapter.calls.Load())
	}
}
