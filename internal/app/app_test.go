package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dylanknuth/raspi-media-player/internal/database"
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
