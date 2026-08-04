package enrichment

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestWikimediaLiveContract(t *testing.T) {
	if os.Getenv("LIVE_METADATA_TEST") != "1" {
		t.Skip("set LIVE_METADATA_TEST=1")
	}
	provider := NewWikimediaProvider("raspi-media-player-live-test/0.1 (https://github.com/dylanknuth/raspi-media-player)", nil)
	result, err := provider.Lookup(context.Background(), TrackHint{Artist: "David Bowie", Title: "Heroes"})
	if err != nil {
		t.Fatal(err)
	}
	if !validImage(result.Image) {
		t.Fatalf("missing valid attributed image: %+v", result)
	}
}
func TestMusicBrainzIdentityTagsAndRelations(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("User-Agent") != "HouseJukebox/1.0 (test@example.test)" {
			t.Errorf("missing user agent")
		}
		if strings.Contains(r.URL.Path, "artist/") && strings.Contains(r.URL.Path, "mbid") {
			return jsonResponse(`{"id":"mbid","name":"Björk","tags":[{"name":"electronic","count":9}],"relations":[{"type":"collaboration","artist":{"id":"related","name":"Tricky"}}]}`), nil
		}
		return jsonResponse(`{"artists":[{"id":"mbid","name":"Björk"}]}`), nil
	})}
	provider := NewMusicBrainzProvider("HouseJukebox/1.0 (test@example.test)", client)
	provider.baseURL = "https://example.test/"
	provider.interval = 0
	result, err := provider.Lookup(context.Background(), TrackHint{Artist: "Bjork", Title: "Joga"})
	if err != nil || result.Hint.Artist != "Björk" || len(result.Genres) != 1 || len(result.RelatedArtists) != 1 || calls != 2 {
		t.Fatalf("result=%+v calls=%d err=%v", result, calls, err)
	}
}
func TestWikimediaImageIncludesLicenseAttribution(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Query().Get("action") {
		case "wbsearchentities":
			return jsonResponse(`{"search":[{"id":"Q1","label":"Björk","description":"Icelandic singer and musician"}]}`), nil
		case "wbgetentities":
			return jsonResponse(`{"entities":{"Q1":{"claims":{"P18":[{"mainsnak":{"datavalue":{"value":"Bjork.jpg"}}}]}}}}`), nil
		default:
			return jsonResponse(`{"query":{"pages":{"1":{"imageinfo":[{"thumburl":"https://upload.wikimedia.org/thumb.jpg","descriptionurl":"https://commons.wikimedia.org/wiki/File:Bjork.jpg","extmetadata":{"Artist":{"value":"Photographer"},"LicenseShortName":{"value":"CC BY-SA 4.0"},"Credit":{"value":"Wikimedia Commons"}}}]}}}}`), nil
		}
	})}
	provider := NewWikimediaProvider("HouseJukebox/1.0", client)
	provider.wikidataURL = "https://example.test/wikidata"
	provider.commonsURL = "https://example.test/commons"
	result, err := provider.Lookup(context.Background(), TrackHint{Artist: "Björk"})
	if err != nil || !validImage(result.Image) || !strings.Contains(result.Image.Attribution, "CC BY-SA") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
func TestCoordinatorMergesProvidersAndPrefersAttributedWikimediaImage(t *testing.T) {
	_ = time.Second
	destination := Result{Hint: TrackHint{Artist: "Artist"}, Image: Image{URL: "https://last.fm/default_artist.png", SourceURL: "https://last.fm/a", Attribution: "Last.fm"}}
	mergeResult(&destination, Result{Provider: "wikimedia", Image: Image{URL: "https://upload.wikimedia.org/photo.jpg", SourceURL: "https://commons.wikimedia.org/wiki/File:Photo.jpg", Attribution: "Photographer · CC BY"}, Genres: []string{"Rock"}})
	if !strings.Contains(destination.Image.URL, "wikimedia") || len(destination.Genres) != 1 {
		t.Fatalf("merged=%+v", destination)
	}
}
