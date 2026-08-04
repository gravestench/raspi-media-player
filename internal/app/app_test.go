package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
)

func testHandler(t *testing.T, logs *bytes.Buffer, options ...Options) http.Handler {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	handler, err := New(logger, db, BuildInfo{Version: "test", Commit: "abc", BuiltAt: "now"}, options...)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

type appMetadataProvider struct{}

func (appMetadataProvider) Name() string { return "test" }
func (appMetadataProvider) Lookup(_ context.Context, h enrichment.TrackHint) (enrichment.Result, error) {
	return enrichment.Result{Hint: h, Provider: "test", Genres: []string{"jazz"}, RelatedArtists: []enrichment.ArtistSummary{{Name: "Related"}}, Status: "ready"}, nil
}

func TestEnabledEnrichmentEndpointBecomesReady(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "metadata.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	coordinator := enrichment.NewCoordinator(enrichment.NewStore(db), time.Hour, appMetadataProvider{})
	var logs bytes.Buffer
	handler, err := New(slog.New(slog.NewJSONHandler(&logs, nil)), db, BuildInfo{}, Options{Enrichment: coordinator})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/enrichment?title=Artist%20-%20Song", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code == http.StatusOK && bytes.Contains(response.Body.Bytes(), []byte(`"status":"ready"`)) && bytes.Contains(response.Body.Bytes(), []byte(`"jazz"`)) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("enrichment did not become ready")
}

func TestEnrichmentImageEndpointRejectsUnknownKey(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ImageCache: enrichment.NewImageCache(t.TempDir(), nil)})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/enrichment/images/not-present", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestHealthAndVersionEndpoints(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	tests := []struct {
		path   string
		status int
		field  string
		value  string
	}{
		{"/api/v1/health/live", http.StatusOK, "status", "ok"},
		{"/api/v1/health/ready", http.StatusOK, "status", "ready"},
		{"/api/v1/version", http.StatusOK, "version", "test"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status: got %d want %d", response.Code, test.status)
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body[test.field] != test.value {
				t.Fatalf("%s: got %q want %q", test.field, body[test.field], test.value)
			}
			if response.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing X-Request-ID")
			}
		})
	}
}

func TestRequestIDIsLogged(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	request.Header.Set("X-Request-ID", "house-test-request")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") != "house-test-request" {
		t.Fatal("request ID was not echoed")
	}
	if !bytes.Contains(logs.Bytes(), []byte(`"request_id":"house-test-request"`)) {
		t.Fatalf("request ID missing from log: %s", logs.String())
	}
}

func TestStaticIndex(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d", response.Code, http.StatusOK)
	}
	if !bytes.Contains(response.Body.Bytes(), []byte("House Jukebox")) {
		t.Fatal("index page content missing")
	}
}

func TestEnrichmentEndpointDegradesWhenProviderDisabled(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/enrichment?title=Artist%20-%20Song", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"disabled"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"artist":"Artist"`)) {
		t.Fatalf("disabled enrichment response: %d %s", response.Code, response.Body.String())
	}
}
