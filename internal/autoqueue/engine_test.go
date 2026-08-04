package autoqueue

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dylanknuth/raspi-media-player/internal/database"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/settings"
	"github.com/dylanknuth/raspi-media-player/internal/youtube"
)

type fakeSearch struct {
	calls   int
	queries []string
}

func (s *fakeSearch) Search(_ context.Context, query string, _ int) ([]youtube.Result, error) {
	s.calls++
	s.queries = append(s.queries, query)
	return []youtube.Result{{ID: fmt.Sprint(s.calls), Title: query, URL: fmt.Sprintf("https://www.youtube.com/watch?v=auto%d", s.calls)}}, nil
}

func TestRefillUsesOnlyActiveUsersAndMaintainsConfiguredDepth(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "autoqueue.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		INSERT INTO users (id, username, username_key, password_hash) VALUES ('active-user', 'Active', 'active', 'hash'), ('idle-user', 'Idle', 'idle', 'hash');
		INSERT INTO sessions (id, user_id, token_hash, csrf_hash, created_at, expires_at, last_seen_at) VALUES
			('active-session', 'active-user', X'01', X'02', CURRENT_TIMESTAMP, datetime('now', '+1 day'), CURRENT_TIMESTAMP),
			('idle-session', 'idle-user', X'03', X'04', datetime('now', '-1 day'), datetime('now', '+1 day'), datetime('now', '-1 day'));
		INSERT INTO playback_history (id, queue_item_id, source_kind, source_url, title, submitter_user_id, started_at, outcome) VALUES
			('h1', 'q1', 'youtube', 'https://example.test/1', 'The Cure - Just Like Heaven', 'active-user', datetime('now', '-1 hour'), 'completed'),
			('h2', 'q2', 'youtube', 'https://example.test/2', 'The Cure - Pictures of You', 'active-user', datetime('now', '-2 hour'), 'completed'),
			('h3', 'q3', 'youtube', 'https://example.test/3', 'Ignored Artist - Song', 'idle-user', datetime('now', '-1 hour'), 'completed');
		INSERT INTO media_enrichments (cache_key, artist, title, genres_json, related_artists_json, status, expires_at) VALUES
			('the-cure', 'The Cure', 'Just Like Heaven', '["post-punk"]', '[]', 'ready', datetime('now', '+1 day'));
	`)
	if err != nil {
		t.Fatal(err)
	}
	definitions := []settings.Definition{
		{Key: "auto_queue_enabled", Value: "true"},
		{Key: "auto_queue_mode", Value: "active_users"},
		{Key: "auto_queue_depth", Value: "3"},
		{Key: "auto_queue_active_seconds", Value: "300"},
		{Key: "queue_limit", Value: "100"},
		{Key: "lastfm_api_key", Secret: true},
	}
	store, err := settings.NewStore(db, definitions, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	search := &fakeSearch{}
	engine := New(db, queuepkg.NewStore(db), store, search, slog.Default(), 100)
	added, err := engine.Refill(context.Background())
	if err != nil || added != 3 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	snapshot, err := queuepkg.NewStore(db).Snapshot(context.Background())
	if err != nil || len(snapshot.Items) != 3 {
		t.Fatalf("snapshot=%+v err=%v", snapshot, err)
	}
	for _, item := range snapshot.Items {
		if !strings.Contains(item.Title, " - ") || item.Submitter.DisplayName != "Auto-queue" || item.Source.Kind != "youtube" {
			t.Fatalf("unexpected auto-queue item: %+v", item)
		}
	}
	added, err = engine.Refill(context.Background())
	if err != nil || added != 0 {
		t.Fatalf("full target added=%d err=%v", added, err)
	}
}

func TestFairActiveUserModeRepresentsEachListener(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "fair.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`
		INSERT INTO users (id, username, username_key, password_hash) VALUES ('u1','One','one','hash'),('u2','Two','two','hash');
		INSERT INTO sessions (id,user_id,token_hash,csrf_hash,created_at,expires_at,last_seen_at) VALUES ('s1','u1',X'01',X'02',CURRENT_TIMESTAMP,datetime('now','+1 day'),CURRENT_TIMESTAMP),('s2','u2',X'03',X'04',CURRENT_TIMESTAMP,datetime('now','+1 day'),CURRENT_TIMESTAMP);
		INSERT INTO playback_history (id,queue_item_id,source_kind,source_url,title,submitter_user_id,started_at,outcome) VALUES ('h1','q1','youtube','https://example.test/1','Artist One - Song','u1',CURRENT_TIMESTAMP,'completed'),('h2','q2','youtube','https://example.test/2','Artist Two - Song','u2',CURRENT_TIMESTAMP,'completed');`)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore(db, []settings.Definition{{Key: "auto_queue_enabled", Value: "true"}, {Key: "auto_queue_mode", Value: "active_users"}, {Key: "auto_queue_depth", Value: "2"}, {Key: "auto_queue_active_seconds", Value: "300"}, {Key: "queue_limit", Value: "100"}, {Key: "lastfm_api_key", Secret: true}}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	added, err := New(db, queuepkg.NewStore(db), store, &fakeSearch{}, slog.Default(), 100).Refill(context.Background())
	if err != nil || added != 2 {
		t.Fatalf("added=%d err=%v", added, err)
	}
	var represented int
	if err := db.QueryRow(`SELECT COUNT(*) FROM auto_queue_user_turns WHERE user_id IN ('u1','u2')`).Scan(&represented); err != nil || represented != 2 {
		t.Fatalf("represented=%d err=%v", represented, err)
	}
}

func TestSpecificSeedModeDoesNotRequireActiveUsers(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "seeds.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := settings.NewStore(db, []settings.Definition{{Key: "auto_queue_enabled", Value: "true"}, {Key: "auto_queue_mode", Value: "specific_seeds"}, {Key: "auto_queue_artists", Value: "Björk"}, {Key: "auto_queue_genres", Value: ""}, {Key: "auto_queue_depth", Value: "1"}, {Key: "queue_limit", Value: "100"}, {Key: "lastfm_api_key", Secret: true}}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	search := &fakeSearch{}
	added, err := New(db, queuepkg.NewStore(db), store, search, slog.Default(), 100).Refill(context.Background())
	if err != nil || added != 1 || len(search.queries) != 1 || search.queries[0] != "Björk music" {
		t.Fatalf("added=%d queries=%v err=%v", added, search.queries, err)
	}
}

func TestRelatedLastModeUsesCachedGenresAndArtists(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "related.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := settings.NewStore(db, []settings.Definition{{Key: "auto_queue_enabled", Value: "true"}, {Key: "auto_queue_mode", Value: "related_last"}, {Key: "auto_queue_depth", Value: "2"}, {Key: "queue_limit", Value: "100"}, {Key: "lastfm_api_key", Secret: true}}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	queue := queuepkg.NewStore(db)
	if _, _, err := queue.AddSourceTitled(context.Background(), "youtube", "https://www.youtube.com/watch?v=seed", "Broadcast - Come On Let's Go", "Test", nil, 100); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO media_enrichments (cache_key,artist,title,genres_json,related_artists_json,status,expires_at) VALUES ('broadcast','Broadcast','Come On Let''s Go','[]','[{"name":"Stereolab"}]','ready',datetime('now','+1 day'))`)
	if err != nil {
		t.Fatal(err)
	}
	search := &fakeSearch{}
	added, err := New(db, queue, store, search, slog.Default(), 100).Refill(context.Background())
	if err != nil || added != 1 || len(search.queries) != 1 || search.queries[0] != "Stereolab music" {
		t.Fatalf("added=%d queries=%v err=%v", added, search.queries, err)
	}
}

func TestRefillDoesNothingWithoutAnActiveUser(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "inactive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := settings.NewStore(db, []settings.Definition{{Key: "auto_queue_enabled", Value: "true"}, {Key: "auto_queue_depth", Value: "2"}, {Key: "auto_queue_active_seconds", Value: "300"}, {Key: "queue_limit", Value: "100"}, {Key: "lastfm_api_key", Secret: true}}, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	added, err := New(db, queuepkg.NewStore(db), store, &fakeSearch{}, slog.Default(), 100).Refill(context.Background())
	if err != nil || added != 0 {
		t.Fatalf("added=%d err=%v", added, err)
	}
}
