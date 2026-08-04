package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/source"
)

const maxQueueRequestBytes = 16 * 1024

type addQueueRequest struct {
	URL         string `json:"url"`
	DisplayName string `json:"display_name"`
}

type reorderQueueRequest struct {
	ItemIDs []string `json:"item_ids"`
}

func (a *application) getQueue(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.queue.Snapshot(r.Context())
	if err != nil {
		a.internalError(w, r, "list queue", err)
		return
	}
	writeSnapshot(w, http.StatusOK, snapshot)
}

func (a *application) addQueueItem(w http.ResponseWriter, r *http.Request) {
	if !a.limiter.Allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many queue submissions")
		return
	}
	var request addQueueRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	request.URL = strings.TrimSpace(request.URL)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	sourceKind, err := a.sources.Classify(request.URL)
	if err != nil {
		if errors.Is(err, source.ErrUnsupported) {
			writeError(w, http.StatusUnprocessableEntity, "unsupported_source", "this source type is not enabled")
			return
		}
		writeError(w, http.StatusUnprocessableEntity, "invalid_url", err.Error())
		return
	}
	if len(request.DisplayName) > 64 {
		writeError(w, http.StatusUnprocessableEntity, "display_name_too_long", "display_name must be at most 64 characters")
		return
	}
	var submitter *queuepkg.UserSubmitter
	if identity := identityFromContext(r.Context()); identity != nil {
		submitter = &queuepkg.UserSubmitter{ID: identity.Session.User.ID, Username: identity.Session.User.Username}
	}
	snapshot, item, err := a.queue.AddSource(r.Context(), sourceKind, request.URL, request.DisplayName, submitter, a.options.QueueLimit)
	if err != nil {
		a.queueError(w, r, err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("queue item added", "queue_revision", snapshot.Revision, "queue_item_id", item.ID, "source_kind", item.Source.Kind, "anonymous_display_name", item.Submitter.DisplayName)
	w.Header().Set("Location", "/api/v1/queue/items/"+item.ID)
	writeSnapshot(w, http.StatusCreated, snapshot)
}

func (a *application) removeQueueItem(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	snapshot, err := a.queue.Remove(r.Context(), r.PathValue("id"), revision)
	if err != nil {
		a.queueError(w, r, err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("queue item removed", "queue_revision", snapshot.Revision, "queue_item_id", r.PathValue("id"))
	writeSnapshot(w, http.StatusOK, snapshot)
}

func (a *application) reorderQueue(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	var request reorderQueueRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	snapshot, err := a.queue.Reorder(r.Context(), request.ItemIDs, revision)
	if err != nil {
		a.queueError(w, r, err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("queue reordered", "queue_revision", snapshot.Revision, "item_count", len(snapshot.Items))
	writeSnapshot(w, http.StatusOK, snapshot)
}

func (a *application) clearQueue(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	snapshot, err := a.queue.Clear(r.Context(), revision)
	if err != nil {
		a.queueError(w, r, err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("queue cleared", "queue_revision", snapshot.Revision)
	writeSnapshot(w, http.StatusOK, snapshot)
}

func (a *application) skipQueueItem(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	snapshot, err := a.queue.Skip(r.Context(), revision)
	if err != nil {
		a.queueError(w, r, err)
		return
	}
	loggerFromContext(r.Context(), a.logger).Info("queue item skipped", "queue_revision", snapshot.Revision)
	writeSnapshot(w, http.StatusOK, snapshot)
}

func writeSnapshot(w http.ResponseWriter, status int, snapshot queuepkg.Snapshot) {
	w.Header().Set("ETag", fmt.Sprintf(`"%d"`, snapshot.Revision))
	writeJSON(w, status, snapshot)
}

func requiredRevision(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), `"`)
	if value == "" {
		writeError(w, http.StatusPreconditionRequired, "revision_required", "If-Match queue revision is required")
		return 0, false
	}
	revision, err := strconv.ParseInt(value, 10, 64)
	if err != nil || revision < 0 {
		writeError(w, http.StatusBadRequest, "invalid_revision", "If-Match must contain a valid queue revision")
		return 0, false
	}
	return revision, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxQueueRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func validateStreamURL(value string) error {
	if value == "" {
		return errors.New("url is required")
	}
	if len(value) > 2048 {
		return errors.New("url must be at most 2048 characters")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("url must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return errors.New("url credentials are not allowed")
	}
	return nil
}

func (a *application) queueError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, queuepkg.ErrDuplicate):
		writeError(w, http.StatusConflict, "duplicate_source", "this source is already in the queue")
	case errors.Is(err, queuepkg.ErrNotFound):
		writeError(w, http.StatusNotFound, "queue_item_not_found", "queue item was not found")
	case errors.Is(err, queuepkg.ErrConflict):
		writeError(w, http.StatusConflict, "revision_conflict", "the queue changed; refresh and try again")
	case errors.Is(err, queuepkg.ErrFull):
		writeError(w, http.StatusConflict, "queue_full", "the queue has reached its configured limit")
	case errors.Is(err, source.ErrUnsupported):
		writeError(w, http.StatusUnprocessableEntity, "unsupported_source", "this source type is not enabled")
	default:
		a.internalError(w, r, "queue operation", err)
	}
}

func (a *application) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	loggerFromContext(r.Context(), a.logger).Error(operation+" failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "an internal error occurred")
}

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]rateWindow
}
type rateWindow struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, clients: make(map[string]rateWindow)}
}
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	current := l.clients[key]
	if current.start.IsZero() || now.Sub(current.start) >= l.window {
		current = rateWindow{start: now}
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.clients[key] = current
	return true
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
