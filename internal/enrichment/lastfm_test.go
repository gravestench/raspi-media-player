package enrichment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLastFMProviderMapsArtistMetadataWithoutLeakingKey(t *testing.T) {
	const apiKey = "test-secret-api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != apiKey || r.URL.Query().Get("artist") != "Björk" {
			t.Errorf("unexpected query")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"artist":{"name":"Björk","url":"https://last.fm/bjork","image":[{"#text":"https://images.example/bjork.jpg","size":"mega"}],"tags":{"tag":[{"name":"electronic"}]},"similar":{"artist":[{"name":"Kate Bush","url":"https://last.fm/kate"}]},"bio":{"summary":"Icelandic artist <a href=\"x\">Read more</a>"}}}`))
	}))
	defer server.Close()
	provider := NewLastFMProvider(apiKey, server.Client())
	provider.baseURL = server.URL
	result, err := provider.Lookup(context.Background(), TrackHint{Artist: "Björk", Title: "Jóga"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != "lastfm" || result.Image.URL == "" || len(result.Genres) != 1 || len(result.RelatedArtists) != 1 || strings.Contains(result.Biography, "<a") {
		t.Fatalf("result: %+v", result)
	}
}

func TestLastFMProviderRequiresKeyAndRedactsTransportErrors(t *testing.T) {
	if _, err := NewLastFMProvider("", nil).Lookup(context.Background(), TrackHint{Artist: "Artist"}); err == nil {
		t.Fatal("missing key accepted")
	}
	provider := NewLastFMProvider("never-print-me", http.DefaultClient)
	provider.baseURL = "http://127.0.0.1:1"
	_, err := provider.Lookup(context.Background(), TrackHint{Artist: "Artist"})
	if err == nil || strings.Contains(err.Error(), "never-print-me") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestLastFMTagDiscoveryMapsArtistsAndTracks(t *testing.T) {
	const apiKey = "tag-discovery-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != apiKey || r.URL.Query().Get("tag") != "spiritual jazz" || r.URL.Query().Get("limit") != "12" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("method") {
		case "tag.getTopArtists":
			_, _ = w.Write([]byte(`{"topartists":{"artist":[{"name":"Alice Coltrane","url":"https://last.fm/alice"}]}}`))
		case "tag.getTopTracks":
			_, _ = w.Write([]byte(`{"tracks":{"track":[{"name":"The Creator Has a Master Plan","url":"https://last.fm/track","artist":{"name":"Pharoah Sanders","url":"https://last.fm/pharoah"}}]}}`))
		default:
			t.Errorf("unexpected method")
		}
	}))
	defer server.Close()
	provider := NewLastFMProvider(apiKey, server.Client())
	provider.baseURL = server.URL
	result, err := provider.DiscoverTag(context.Background(), "spiritual jazz", 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artists) != 1 || result.Artists[0].Name != "Alice Coltrane" || len(result.Tracks) != 1 || result.Tracks[0].Artist != "Pharoah Sanders" {
		t.Fatalf("result=%+v", result)
	}
}
