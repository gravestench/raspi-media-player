package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/database"
	"github.com/dylanknuth/raspi-media-player/internal/settings"
)

func TestVoteManagerCountsActiveListenersAndExpiresVotes(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "votes.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := settings.NewStore(db, []settings.Definition{{Key: "vote_enabled", Value: "true"}, {Key: "vote_active_seconds", Value: "60"}, {Key: "vote_timeout_seconds", Value: "90"}, {Key: "vote_percent", Value: "75"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	manager := newVoteManager(store)
	manager.touch("one")
	manager.touch("two")
	state := manager.setVote(context.Background(), "skip:track", "track", "one", true)
	if state.ActiveListeners != 2 || state.Required != 2 || state.Votes != 1 || !state.Voted {
		t.Fatalf("state=%+v", state)
	}
	manager.mu.Lock()
	manager.votes["skip:track"]["one"] = time.Now().Add(-2 * time.Minute)
	manager.mu.Unlock()
	state = manager.state(context.Background(), "skip:track", "track", "one")
	if state.Votes != 0 || state.Voted {
		t.Fatalf("expired vote remained: %+v", state)
	}
}

func TestRemovingAnotherUsersQueueItemRequiresVote(t *testing.T) {
	var logs bytes.Buffer
	handler := testHandler(t, &logs, Options{ArgonMemory: 1024, ArgonIterations: 1, AuthRate: 30, Settings: []settings.Definition{
		{Key: "vote_enabled", Value: "true"}, {Key: "vote_active_seconds", Value: "60"}, {Key: "vote_timeout_seconds", Value: "90"}, {Key: "vote_percent", Value: "75"},
	}})
	createUser := func(username string) (authResponse, []*http.Cookie) {
		response := authRequest(t, handler, http.MethodPost, "/api/v1/auth/signup", `{"username":"`+username+`","password":"household-password","password_confirmation":"household-password"}`, nil, "")
		return decodeAuth(t, response), sessionCookies(response)
	}
	owner, ownerCookies := createUser("queue_owner")
	listenerOne, oneCookies := createUser("listener_one")
	listenerTwo, twoCookies := createUser("listener_two")
	queued := authRequest(t, handler, http.MethodPost, "/api/v1/queue/items", `{"url":"https://example.com/shared.mp3","title":"Shared Track"}`, ownerCookies, owner.CSRFToken)
	var snapshot struct {
		Revision int64 `json:"revision"`
		Items    []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(queued.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	itemID := snapshot.Items[0].ID
	authRequest(t, handler, http.MethodGet, "/api/v1/queue", "", oneCookies, "")
	authRequest(t, handler, http.MethodGet, "/api/v1/queue", "", twoCookies, "")
	remove := func(cookies []*http.Cookie, csrf string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/queue/items/"+itemID, nil)
		request.Header.Set("If-Match", `"1"`)
		request.Header.Set("X-CSRF-Token", csrf)
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	first := remove(oneCookies, listenerOne.CSRFToken)
	if first.Code != http.StatusOK {
		t.Fatalf("first vote: %d %s", first.Code, first.Body.String())
	}
	var firstState struct {
		Revision int64 `json:"revision"`
		Items    []struct {
			RemovalVote *struct {
				Votes, Required int
				Voted           bool
			} `json:"removal_vote"`
		} `json:"items"`
	}
	if err := json.NewDecoder(first.Body).Decode(&firstState); err != nil {
		t.Fatal(err)
	}
	if firstState.Revision != 1 || len(firstState.Items) != 1 || firstState.Items[0].RemovalVote == nil || firstState.Items[0].RemovalVote.Votes != 1 || firstState.Items[0].RemovalVote.Required != 2 || !firstState.Items[0].RemovalVote.Voted {
		t.Fatalf("first state: %+v", firstState)
	}
	second := remove(twoCookies, listenerTwo.CSRFToken)
	if second.Code != http.StatusOK || bytes.Contains(second.Body.Bytes(), []byte(itemID)) {
		t.Fatalf("threshold removal: %d %s", second.Code, second.Body.String())
	}
}
