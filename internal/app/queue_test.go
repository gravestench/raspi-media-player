package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
)

func queueRequest(t *testing.T, handler http.Handler, method, path, revision string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if revision != "" {
		request.Header.Set("If-Match", revision)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func snapshotFrom(t *testing.T, response *httptest.ResponseRecorder) queuepkg.Snapshot {
	t.Helper()
	var snapshot queuepkg.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestAnonymousQueueLifecycle(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)

	first := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", `{"url":"https://netcast.kfjc.org/kfjc-128k-mp3","title":"Artist - Track","display_name":"Living room"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("add first: %d %s", first.Code, first.Body.String())
	}
	snapshot := snapshotFrom(t, first)
	if snapshot.Revision != 1 || len(snapshot.Items) != 1 || snapshot.Items[0].Title != "Artist - Track" || snapshot.Items[0].Status != "queued" || snapshot.Items[0].Submitter.Kind != "anonymous" || !snapshot.Items[0].Radio {
		t.Fatalf("unexpected first snapshot: %+v", snapshot)
	}
	firstID := snapshot.Items[0].ID

	second := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", `{"url":"https://example.com/music.mp3"}`)
	if second.Code != http.StatusCreated {
		t.Fatalf("add second: %d %s", second.Code, second.Body.String())
	}
	snapshot = snapshotFrom(t, second)
	secondID := snapshot.Items[1].ID

	reordered := queueRequest(t, handler, http.MethodPut, "/api/v1/queue/order", `"2"`, `{"item_ids":["`+secondID+`","`+firstID+`"]}`)
	if reordered.Code != http.StatusOK {
		t.Fatalf("reorder: %d %s", reordered.Code, reordered.Body.String())
	}
	snapshot = snapshotFrom(t, reordered)
	if snapshot.Items[0].ID != secondID || snapshot.Revision != 3 {
		t.Fatalf("unexpected reorder: %+v", snapshot)
	}

	conflict := queueRequest(t, handler, http.MethodDelete, "/api/v1/queue/items/"+firstID, `"2"`, "")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("stale revision: got %d", conflict.Code)
	}

	removed := queueRequest(t, handler, http.MethodDelete, "/api/v1/queue/items/"+firstID, `"3"`, "")
	if removed.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", removed.Code, removed.Body.String())
	}
	snapshot = snapshotFrom(t, removed)
	if len(snapshot.Items) != 1 || snapshot.Items[0].Position != 0 {
		t.Fatalf("queue was not compacted: %+v", snapshot)
	}

	skipped := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/skip", `"4"`, "")
	if skipped.Code != http.StatusOK || len(snapshotFrom(t, skipped).Items) != 0 {
		t.Fatalf("skip: %d %s", skipped.Code, skipped.Body.String())
	}
}

func TestQueueValidationAndDuplicate(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	invalid := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", `{"url":"file:///etc/passwd"}`)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid URL: %d", invalid.Code)
	}
	validBody := `{"url":"https://example.com/a.mp3"}`
	if response := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", validBody); response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	duplicate := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", validBody)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", duplicate.Code)
	}
	missingRevision := queueRequest(t, handler, http.MethodDelete, "/api/v1/queue", "", "")
	if missingRevision.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing revision: %d", missingRevision.Code)
	}
}

func TestQueueLimitAndRateLimit(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{QueueLimit: 1, QueueRate: 2})
	if response := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", `{"url":"https://example.com/one.mp3"}`); response.Code != http.StatusCreated {
		t.Fatal(response.Body.String())
	}
	full := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", `{"url":"https://example.com/two.mp3"}`)
	if full.Code != http.StatusConflict {
		t.Fatalf("queue limit: %d %s", full.Code, full.Body.String())
	}
	limited := queueRequest(t, handler, http.MethodPost, "/api/v1/queue/items", "", `{"url":"https://example.com/three.mp3"}`)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit: %d %s", limited.Code, limited.Body.String())
	}
}
