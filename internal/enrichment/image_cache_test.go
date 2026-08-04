package enrichment

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
)

func TestImageCacheStoresValidatedAttributedImage(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 200, 160))
	canvas.Set(0, 0, color.Black)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("User-Agent"); got != "raspi-media-player/test" {
			t.Errorf("User-Agent = %q", got)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(encoded.Bytes()))}, nil
	})}
	cache := NewImageCache(t.TempDir(), client).WithUserAgent("raspi-media-player/test")
	value, err := cache.Cache(context.Background(), "abc123", Image{URL: "https://upload.wikimedia.org/photo.png", SourceURL: "https://commons.wikimedia.org/wiki/File:Photo.png", Attribution: "Photographer · CC BY"})
	if err != nil || value.URL != "/api/v1/enrichment/images/abc123" {
		t.Fatalf("cache=%+v err=%v", value, err)
	}
	data, mime, err := cache.Read("abc123")
	if err != nil || mime != "image/png" || len(data) == 0 {
		t.Fatalf("read mime=%q bytes=%d err=%v", mime, len(data), err)
	}
}

type wikimediaImageProvider struct{}

func (wikimediaImageProvider) Name() string { return "wikimedia" }
func (wikimediaImageProvider) Lookup(_ context.Context, hint TrackHint) (Result, error) {
	return Result{Hint: hint, Provider: "wikimedia", Image: Image{
		URL:         "https://upload.wikimedia.org/photo.png",
		SourceURL:   "https://commons.wikimedia.org/wiki/File:Photo.png",
		Attribution: "Photographer · CC BY",
	}}, nil
}

func TestCoordinatorRewritesWikimediaImageToLocalCache(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 200, 160))
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(encoded.Bytes()))}, nil
	})}
	db, err := database.Open(filepath.Join(t.TempDir(), "cache.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	coordinator := NewCoordinator(NewStore(db), time.Hour, wikimediaImageProvider{}).WithImageCache(NewImageCache(t.TempDir(), client))
	_, _ = coordinator.Lookup(context.Background(), "Artist - Song")
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, err := coordinator.Lookup(context.Background(), "Artist - Song")
		if err == nil && strings.HasPrefix(got.Image.URL, "/api/v1/enrichment/images/") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("image URL was not rewritten to local cache")
}

func TestImageValidationRejectsUnsafeOrPlaceholderURLs(t *testing.T) {
	for _, value := range []Image{{URL: "http://example.com/a.jpg", SourceURL: "https://source", Attribution: "credit"}, {URL: "https://last.fm/2a96cbd8b46e442fc41c2b86b821562f.png", SourceURL: "https://source", Attribution: "credit"}, {URL: "https://example.com/a.jpg", SourceURL: "", Attribution: ""}} {
		if validImage(value) {
			t.Errorf("accepted image %+v", value)
		}
	}
	if !strings.HasPrefix("https://upload.wikimedia.org/a.jpg", "https://") {
		t.Fatal("unreachable")
	}
}
