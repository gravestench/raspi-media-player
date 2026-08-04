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
	Title       string `json:"title"`
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
	a.attachVote(r, &snapshot)
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
	request.Title = strings.TrimSpace(request.Title)
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
	if len(request.Title) > 240 {
		writeError(w, http.StatusUnprocessableEntity, "title_too_long", "title must be at most 240 characters")
		return
	}
	var submitter *queuepkg.UserSubmitter
	if identity := identityFromContext(r.Context()); identity != nil {
		submitter = &queuepkg.UserSubmitter{ID: identity.Session.User.ID, Username: identity.Session.User.Username}
	}
	snapshot, item, err := a.queue.AddSourceTitled(r.Context(), sourceKind, request.URL, request.Title, request.DisplayName, submitter, a.options.QueueLimit)
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
	itemID := r.PathValue("id")
	current, err := a.queue.Snapshot(r.Context())
	if err != nil {
		a.internalError(w, r, "read removal vote state", err)
		return
	}
	var target *queuepkg.Item
	for index := range current.Items {
		if current.Items[index].ID == itemID {
			target = &current.Items[index]
			break
		}
	}
	if target == nil {
		writeError(w, http.StatusNotFound, "queue_item_not_found", "queue item was not found")
		return
	}
	if target.Default { writeError(w, http.StatusConflict, "default_radio_protected", "the default radio is managed from Admin settings"); return }
	identity := identityFromContext(r.Context())
	owned := identity != nil && target.Submitter.UserID == identity.Session.User.ID
	admin := identity != nil && identity.Session.User.IsAdmin
	policy := a.votes.policy(r.Context())
	if policy.enabled && !owned && !admin {
		state := a.votes.setVote(r.Context(), "remove:"+itemID, itemID, listenerID(r), true)
		if state.Votes < state.Required {
			a.attachVote(r, &current)
			loggerFromContext(r.Context(), a.logger).Info("queue removal vote changed", "queue_item_id", itemID, "votes", state.Votes, "required", state.Required)
			writeSnapshot(w, http.StatusOK, current)
			return
		}
	}
	snapshot, err := a.queue.Remove(r.Context(), itemID, revision)
	if err != nil {
		a.queueError(w, r, err)
		return
	}
	a.votes.clear("remove:" + itemID)
	a.attachVote(r, &snapshot)
	loggerFromContext(r.Context(), a.logger).Info("queue item removed", "queue_revision", snapshot.Revision, "queue_item_id", itemID)
	writeSnapshot(w, http.StatusOK, snapshot)
}

func (a *application) withdrawRemovalVote(w http.ResponseWriter, r *http.Request) {
	if _, ok := requiredRevision(w, r); !ok {
		return
	}
	snapshot, err := a.queue.Snapshot(r.Context())
	if err != nil {
		a.internalError(w, r, "read removal vote state", err)
		return
	}
	itemID := r.PathValue("id")
	found := false
	for _, item := range snapshot.Items {
		if item.ID == itemID {
			found = true
			break
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "queue_item_not_found", "queue item was not found")
		return
	}
	a.votes.setVote(r.Context(), "remove:"+itemID, itemID, listenerID(r), false)
	a.attachVote(r, &snapshot)
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
	a.voteToSkip(w, r, true)
}

func (a *application) withdrawSkipVote(w http.ResponseWriter, r *http.Request) {
	a.voteToSkip(w, r, false)
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
	case errors.Is(err, queuepkg.ErrProtected):
		writeError(w, http.StatusConflict, "default_radio_protected", "the default radio is managed from Admin settings")
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
