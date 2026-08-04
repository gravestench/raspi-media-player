package app

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/settings"
)

type voteManager struct {
	mu       sync.Mutex
	settings *settings.Store
	active   map[string]time.Time
	votes    map[string]map[string]time.Time
}

type votePolicy struct {
	enabled      bool
	activeWindow time.Duration
	timeout      time.Duration
	percent      int
}

func newVoteManager(store *settings.Store) *voteManager {
	return &voteManager{settings: store, active: map[string]time.Time{}, votes: map[string]map[string]time.Time{}}
}

func (m *voteManager) policy(ctx context.Context) votePolicy {
	policy := votePolicy{activeWindow: 60 * time.Second, timeout: 90 * time.Second, percent: 50}
	if value, err := m.settings.Value(ctx, "vote_enabled"); err == nil {
		policy.enabled = value == "true"
	}
	if value, err := m.settings.Value(ctx, "vote_active_seconds"); err == nil {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed >= 10 && parsed <= 3600 {
			policy.activeWindow = time.Duration(parsed) * time.Second
		}
	}
	if value, err := m.settings.Value(ctx, "vote_timeout_seconds"); err == nil {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed >= 10 && parsed <= 3600 {
			policy.timeout = time.Duration(parsed) * time.Second
		}
	}
	if value, err := m.settings.Value(ctx, "vote_percent"); err == nil {
		if parsed, parseErr := strconv.Atoi(value); parseErr == nil && parsed >= 1 && parsed <= 100 {
			policy.percent = parsed
		}
	}
	return policy
}

func listenerID(r *http.Request) string {
	if identity := identityFromContext(r.Context()); identity != nil {
		return "user:" + identity.Session.User.ID
	}
	if cookie, err := r.Cookie(listenerCookieName); err == nil && cookie.Value != "" {
		return "browser:" + cookie.Value
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return "request:" + host
}

func (m *voteManager) touch(id string) { m.mu.Lock(); m.active[id] = time.Now(); m.mu.Unlock() }

func (m *voteManager) state(ctx context.Context, voteKey, itemID, listener string) queuepkg.SkipVoteState {
	policy := m.policy(ctx)
	state := queuepkg.SkipVoteState{Enabled: policy.enabled, CurrentItemID: itemID}
	if !policy.enabled || itemID == "" {
		return state
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, seen := range m.active {
		if now.Sub(seen) > policy.activeWindow {
			delete(m.active, id)
		}
	}
	for trackedItem, trackedVotes := range m.votes {
		for id, votedAt := range trackedVotes {
			if now.Sub(votedAt) > policy.timeout {
				delete(trackedVotes, id)
			}
		}
		if len(trackedVotes) == 0 {
			delete(m.votes, trackedItem)
		}
	}
	votes := m.votes[voteKey]
	for id, votedAt := range votes {
		if now.Sub(votedAt) > policy.timeout {
			delete(votes, id)
		}
	}
	state.ActiveListeners = len(m.active)
	if state.ActiveListeners < 1 {
		state.ActiveListeners = 1
	}
	state.Required = int(math.Ceil(float64(state.ActiveListeners*policy.percent) / 100))
	if state.Required < 1 {
		state.Required = 1
	}
	state.Votes = len(votes)
	_, state.Voted = votes[listener]
	if state.Votes > 0 {
		latest := now
		for _, votedAt := range votes {
			if votedAt.Before(latest) {
				latest = votedAt
			}
		}
		state.ExpiresAt = latest.Add(policy.timeout).UTC().Format(time.RFC3339Nano)
	}
	return state
}

func (m *voteManager) setVote(ctx context.Context, voteKey, itemID, listener string, vote bool) queuepkg.SkipVoteState {
	m.touch(listener)
	m.mu.Lock()
	if m.votes[voteKey] == nil {
		m.votes[voteKey] = map[string]time.Time{}
	}
	if vote {
		m.votes[voteKey][listener] = time.Now()
	} else {
		delete(m.votes[voteKey], listener)
	}
	m.mu.Unlock()
	return m.state(ctx, voteKey, itemID, listener)
}

func (m *voteManager) clear(itemID string) { m.mu.Lock(); delete(m.votes, itemID); m.mu.Unlock() }

func (a *application) attachVote(r *http.Request, snapshot *queuepkg.Snapshot) {
	listener := listenerID(r)
	a.votes.touch(listener)
	itemID := snapshot.Playback.CurrentItemID
	if itemID == "" && len(snapshot.Items) > 0 {
		itemID = snapshot.Items[0].ID
	}
	state := a.votes.state(r.Context(), "skip:"+itemID, itemID, listener)
	for _, item := range snapshot.Items {
		if item.ID == itemID && item.Default {
			state.Enabled = false
			break
		}
	}
	snapshot.SkipVote = &state
	identity := identityFromContext(r.Context())
	for index := range snapshot.Items {
		item := &snapshot.Items[index]
		if item.Default {
			continue
		}
		if identity != nil && (identity.Session.User.IsAdmin || item.Submitter.UserID == identity.Session.User.ID) {
			continue
		}
		removal := a.votes.state(r.Context(), "remove:"+item.ID, item.ID, listener)
		if removal.Enabled {
			item.RemovalVote = &removal
		}
	}
}

func (a *application) voteToSkip(w http.ResponseWriter, r *http.Request, vote bool) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	snapshot, err := a.queue.Snapshot(r.Context())
	if err != nil {
		a.internalError(w, r, "read skip vote state", err)
		return
	}
	identity := identityFromContext(r.Context())
	policy := a.votes.policy(r.Context())
	itemID := snapshot.Playback.CurrentItemID
	if itemID == "" && len(snapshot.Items) > 0 {
		itemID = snapshot.Items[0].ID
	}
	for _, item := range snapshot.Items {
		if item.ID == itemID && item.Default {
			writeError(w, http.StatusConflict, "default_radio_protected", "the default radio cannot be skipped; add something to the queue instead")
			return
		}
	}
	if !policy.enabled || (identity != nil && identity.Session.User.IsAdmin) {
		snapshot, err = a.queue.Skip(r.Context(), revision)
		if err != nil {
			a.queueError(w, r, err)
			return
		}
		a.votes.clear("skip:" + itemID)
		writeSnapshot(w, http.StatusOK, snapshot)
		return
	}
	if itemID == "" {
		writeError(w, http.StatusConflict, "nothing_playing", "there is no current item to skip")
		return
	}
	state := a.votes.setVote(r.Context(), "skip:"+itemID, itemID, listenerID(r), vote)
	if vote && state.Votes >= state.Required {
		snapshot, err = a.queue.Skip(r.Context(), revision)
		if err != nil {
			a.queueError(w, r, err)
			return
		}
		a.votes.clear("skip:" + itemID)
		a.attachVote(r, &snapshot)
		loggerFromContext(r.Context(), a.logger).Info("skip vote threshold reached", "queue_item_id", itemID, "votes", state.Votes, "required", state.Required)
		writeSnapshot(w, http.StatusOK, snapshot)
		return
	}
	snapshot.SkipVote = &state
	loggerFromContext(r.Context(), a.logger).Info("skip vote changed", "queue_item_id", itemID, "voted", vote, "votes", state.Votes, "required", state.Required)
	writeSnapshot(w, http.StatusOK, snapshot)
}
