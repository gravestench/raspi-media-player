package app

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEventStreamSendsQueueSnapshot(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	response := newStreamResponse()
	done := make(chan struct{})
	go func() { handler.ServeHTTP(response, request); close(done) }()
	select {
	case <-response.flushed:
	case <-ctx.Done():
		t.Fatal("event stream did not flush")
	}
	if response.statusCode() != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("event response: %d %q", response.statusCode(), response.Header().Get("Content-Type"))
	}
	scanner := bufio.NewScanner(strings.NewReader(response.contents()))
	foundEvent, foundData := false, false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "event: queue" {
			foundEvent = true
		}
		if strings.HasPrefix(line, "data: {") && strings.Contains(line, `"revision":0`) {
			foundData = true
		}
		if foundEvent && foundData {
			cancel()
			<-done
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}
	t.Fatalf("missing initial queue event: event=%v data=%v", foundEvent, foundData)
}

type streamResponse struct {
	mu      sync.Mutex
	header  http.Header
	status  int
	body    strings.Builder
	flushed chan struct{}
	once    sync.Once
}

func newStreamResponse() *streamResponse {
	return &streamResponse{header: make(http.Header), flushed: make(chan struct{})}
}
func (w *streamResponse) Header() http.Header { return w.header }
func (w *streamResponse) WriteHeader(status int) {
	w.mu.Lock()
	if w.status == 0 {
		w.status = status
	}
	w.mu.Unlock()
}
func (w *streamResponse) Write(value []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return io.WriteString(&w.body, string(value))
}
func (w *streamResponse) Flush()           { w.once.Do(func() { close(w.flushed) }) }
func (w *streamResponse) contents() string { w.mu.Lock(); defer w.mu.Unlock(); return w.body.String() }
func (w *streamResponse) statusCode() int  { w.mu.Lock(); defer w.mu.Unlock(); return w.status }

func TestHouseholdInterfaceIncludesAnonymousAndAccountFlows(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	page := response.Body.String()
	for _, expected := range []string{"What should we hear next?", "No account required", "The house queue", "OPTIONAL ACCOUNT", "Confirm your password", "aria-live"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("interface missing %q", expected)
		}
	}
}
