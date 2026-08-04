package app

import (
	"context"
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
	state := manager.setVote(context.Background(), "track", "one", true)
	if state.ActiveListeners != 2 || state.Required != 2 || state.Votes != 1 || !state.Voted {
		t.Fatalf("state=%+v", state)
	}
	manager.mu.Lock()
	manager.votes["track"]["one"] = time.Now().Add(-2 * time.Minute)
	manager.mu.Unlock()
	state = manager.state(context.Background(), "track", "one")
	if state.Votes != 0 || state.Voted {
		t.Fatalf("expired vote remained: %+v", state)
	}
}
