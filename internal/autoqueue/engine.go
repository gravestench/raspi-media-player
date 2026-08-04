package autoqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dylanknuth/raspi-media-player/internal/enrichment"
	queuepkg "github.com/dylanknuth/raspi-media-player/internal/queue"
	"github.com/dylanknuth/raspi-media-player/internal/settings"
	"github.com/dylanknuth/raspi-media-player/internal/youtube"
)

type Engine struct {
	db         *sql.DB
	queue      *queuepkg.Store
	settings   *settings.Store
	youtube    youtube.Searcher
	logger     *slog.Logger
	queueLimit int
	mu         sync.Mutex
	random     *rand.Rand
}

type preference struct {
	Kind   string
	Name   string
	Weight int
}

func New(db *sql.DB, queue *queuepkg.Store, store *settings.Store, searcher youtube.Searcher, logger *slog.Logger, queueLimit int) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{db: db, queue: queue, settings: store, youtube: searcher, logger: logger, queueLimit: queueLimit, random: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	e.refillLogged(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.refillLogged(ctx)
		}
	}
}

func (e *Engine) refillLogged(ctx context.Context) {
	added, err := e.Refill(ctx)
	if err != nil {
		e.logger.WarnContext(ctx, "auto-queue refill failed", "error", err)
		return
	}
	if added > 0 {
		e.logger.InfoContext(ctx, "auto-queue refilled", "items_added", added)
	}
}

func (e *Engine) Refill(ctx context.Context) (int, error) {
	if e.youtube == nil || !e.boolSetting(ctx, "auto_queue_enabled", false) || !e.boolSetting(ctx, "youtube_search_enabled", true) {
		return 0, nil
	}
	depth := e.intSetting(ctx, "auto_queue_depth", 3, 1, 20)
	activeSeconds := e.intSetting(ctx, "auto_queue_active_seconds", 300, 30, 3600)
	snapshot, err := e.queue.Snapshot(ctx)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, item := range snapshot.Items {
		if item.Status == "queued" && !item.Default {
			queued++
		}
	}
	needed := depth - queued
	if needed <= 0 {
		return 0, nil
	}
	mode := e.stringSetting(ctx, "auto_queue_mode", "active_users")
	activeSince := time.Now().Add(-time.Duration(activeSeconds) * time.Second)
	limit := e.intSetting(ctx, "queue_limit", e.queueLimit, 1, 10000)
	added := 0
	for attempts := 0; added < needed && attempts < needed*6; attempts++ {
		preferences, selectedUser, err := e.preferencesForMode(ctx, mode, activeSince, snapshot)
		if err != nil {
			return added, err
		}
		if len(preferences) == 0 {
			break
		}
		preference := e.pick(preferences)
		query := preference.Name
		canonicalTitle := ""
		if preference.Kind == "genre" {
			query += " music"
			if key, keyErr := e.settings.Value(ctx, "lastfm_api_key"); keyErr == nil && key != "" {
				discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				discovery, discoverErr := enrichment.NewLastFMProvider(key, nil).DiscoverTag(discoveryCtx, preference.Name, 50)
				cancel()
				if discoverErr == nil && len(discovery.Tracks) > 0 {
					track := discovery.Tracks[e.randomIndex(len(discovery.Tracks))]
					query = track.Artist + " " + track.Name
					canonicalTitle = track.Artist + " - " + track.Name
				}
			}
		} else {
			query += " music"
		}
		searchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		results, searchErr := e.youtube.Search(searchCtx, query, 12)
		cancel()
		if searchErr != nil || len(results) == 0 {
			continue
		}
		result := results[e.randomIndex(len(results))]
		if canonicalTitle == "" {
			canonicalTitle = result.Title
			if enrichment.ParseTitle(result.Title).Artist == "" {
				artist := preference.Name
				if preference.Kind == "genre" && result.Channel != "" {
					artist = result.Channel
				}
				canonicalTitle = artist + " - " + result.Title
			}
		}
		_, _, addErr := e.queue.AddSourceTitled(ctx, "youtube", result.URL, canonicalTitle, "Auto-queue", nil, limit)
		if addErr == nil {
			added++
			if selectedUser != "" {
				if err := e.markUserSelected(ctx, selectedUser); err != nil {
					return added, err
				}
			}
			continue
		}
		if !errors.Is(addErr, queuepkg.ErrDuplicate) && !errors.Is(addErr, queuepkg.ErrFull) {
			return added, addErr
		}
		if errors.Is(addErr, queuepkg.ErrFull) {
			break
		}
	}
	return added, nil
}

func (e *Engine) preferencesForMode(ctx context.Context, mode string, activeSince time.Time, snapshot queuepkg.Snapshot) ([]preference, string, error) {
	switch mode {
	case "specific_seeds":
		return seedPreferences(e.stringSetting(ctx, "auto_queue_artists", ""), e.stringSetting(ctx, "auto_queue_genres", "")), "", nil
	case "related_last":
		values, err := e.relatedPreferences(ctx, snapshot)
		return values, "", err
	default:
		users, err := e.activeUsers(ctx, activeSince)
		if err != nil || len(users) == 0 {
			return nil, "", err
		}
		// Users without usable history still receive a turn, then the next refill
		// attempt advances to another listener instead of letting them block the queue.
		for range users {
			userID, err := e.fairUser(ctx, users)
			if err != nil {
				return nil, "", err
			}
			values, err := e.userPreferences(ctx, userID)
			if err != nil {
				return nil, "", err
			}
			if len(values) > 0 {
				return values, userID, nil
			}
			if err := e.markUserSelected(ctx, userID); err != nil {
				return nil, "", err
			}
		}
		return nil, "", nil
	}
}

func (e *Engine) activeUsers(ctx context.Context, activeSince time.Time) ([]string, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT DISTINCT user_id FROM sessions WHERE revoked_at IS NULL AND datetime(expires_at) > CURRENT_TIMESTAMP AND datetime(last_seen_at) >= datetime(?)`, activeSince.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		users = append(users, id)
	}
	return users, rows.Err()
}

func (e *Engine) fairUser(ctx context.Context, users []string) (string, error) {
	turns := map[string]string{}
	rows, err := e.db.QueryContext(ctx, `SELECT user_id, last_selected_at FROM auto_queue_user_turns`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var id, at string
		if err := rows.Scan(&id, &at); err != nil {
			rows.Close()
			return "", err
		}
		turns[id] = at
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	oldest := ""
	var candidates []string
	for _, id := range users {
		at := turns[id]
		if len(candidates) == 0 || at < oldest {
			oldest = at
			candidates = []string{id}
		} else if at == oldest {
			candidates = append(candidates, id)
		}
	}
	return candidates[e.randomIndex(len(candidates))], nil
}

func (e *Engine) markUserSelected(ctx context.Context, userID string) error {
	_, err := e.db.ExecContext(ctx, `INSERT INTO auto_queue_user_turns (user_id, last_selected_at) VALUES (?, ?) ON CONFLICT(user_id) DO UPDATE SET last_selected_at = excluded.last_selected_at`, userID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (e *Engine) userPreferences(ctx context.Context, userID string) ([]preference, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT title FROM (
			SELECT title, started_at AS associated_at FROM playback_history WHERE submitter_user_id = ? AND title <> ''
			UNION ALL
			SELECT title, created_at AS associated_at FROM track_likes WHERE user_id = ? AND title <> ''
		) ORDER BY associated_at DESC LIMIT 500`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	artists := make([]string, 0, 200)
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return nil, err
		}
		artist := strings.TrimSpace(enrichment.ParseTitle(title).Artist)
		if artist != "" {
			artists = append(artists, artist)
		}
	}
	if err := rows.Err(); err != nil || len(artists) == 0 {
		return nil, err
	}
	genresByArtist, err := e.genresByArtist(ctx)
	if err != nil {
		return nil, err
	}
	artistCounts := map[string]int{}
	genreCounts := map[string]int{}
	labels := map[string]string{}
	for _, artist := range artists {
		key := strings.ToLower(artist)
		labels[key] = artist
		artistCounts[key]++
		for _, genre := range genresByArtist[key] {
			genreKey := strings.ToLower(genre)
			labels["genre:"+genreKey] = genre
			genreCounts[genreKey]++
		}
	}
	merged := map[string]int{}
	mergeNormalized(merged, "artist:", artistCounts)
	mergeNormalized(merged, "genre:", genreCounts)
	result := make([]preference, 0, len(merged))
	for key, weight := range merged {
		kind, nameKey, _ := strings.Cut(key, ":")
		labelKey := nameKey
		if kind == "genre" {
			labelKey = "genre:" + nameKey
		}
		result = append(result, preference{Kind: kind, Name: labels[labelKey], Weight: weight})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Weight > result[j].Weight })
	if len(result) > 40 {
		result = result[:40]
	}
	return result, nil
}

func seedPreferences(artists, genres string) []preference {
	var result []preference
	for _, entry := range splitSeeds(artists) {
		result = append(result, preference{Kind: "artist", Name: entry, Weight: 10})
	}
	for _, entry := range splitSeeds(genres) {
		result = append(result, preference{Kind: "genre", Name: entry, Weight: 10})
	}
	return result
}

func splitSeeds(value string) []string {
	seen := map[string]bool{}
	var result []string
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' }) {
		part = strings.TrimSpace(part)
		key := strings.ToLower(part)
		if part != "" && !seen[key] {
			seen[key] = true
			result = append(result, part)
		}
	}
	return result
}

func (e *Engine) relatedPreferences(ctx context.Context, snapshot queuepkg.Snapshot) ([]preference, error) {
	if len(snapshot.Items) == 0 {
		return nil, nil
	}
	hint := enrichment.ParseTitle(snapshot.Items[len(snapshot.Items)-1].Title)
	if hint.Artist == "" {
		return nil, nil
	}
	var genresJSON, relatedJSON string
	err := e.db.QueryRowContext(ctx, `SELECT genres_json, related_artists_json FROM media_enrichments WHERE lower(artist) = lower(?) AND status = 'ready' ORDER BY updated_at DESC LIMIT 1`, hint.Artist).Scan(&genresJSON, &relatedJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var genres []string
	var related []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal([]byte(genresJSON), &genres)
	_ = json.Unmarshal([]byte(relatedJSON), &related)
	var values []preference
	for _, name := range genres {
		if strings.TrimSpace(name) != "" {
			values = append(values, preference{Kind: "genre", Name: name, Weight: 10})
		}
	}
	for _, artist := range related {
		if strings.TrimSpace(artist.Name) != "" {
			values = append(values, preference{Kind: "artist", Name: artist.Name, Weight: 10})
		}
	}
	return values, nil
}

func (e *Engine) genresByArtist(ctx context.Context) (map[string][]string, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT artist, genres_json FROM media_enrichments WHERE status = 'ready' AND artist <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]string{}
	for rows.Next() {
		var artist, encoded string
		if err := rows.Scan(&artist, &encoded); err != nil {
			return nil, err
		}
		var genres []string
		if json.Unmarshal([]byte(encoded), &genres) == nil {
			result[strings.ToLower(strings.TrimSpace(artist))] = genres
		}
	}
	return result, rows.Err()
}

func mergeNormalized(destination map[string]int, prefix string, counts map[string]int) {
	maximum := 0
	for _, count := range counts {
		if count > maximum {
			maximum = count
		}
	}
	if maximum == 0 {
		return
	}
	for name, count := range counts {
		destination[prefix+name] += 1 + (count * 10 / maximum)
	}
}

func (e *Engine) pick(values []preference) preference {
	total := 0
	for _, value := range values {
		total += value.Weight
	}
	choice := e.randomIndex(total)
	for _, value := range values {
		choice -= value.Weight
		if choice < 0 {
			return value
		}
	}
	return values[len(values)-1]
}

func (e *Engine) randomIndex(limit int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.random.Intn(limit)
}

func (e *Engine) boolSetting(ctx context.Context, key string, fallback bool) bool {
	value, err := e.settings.Value(ctx, key)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func (e *Engine) stringSetting(ctx context.Context, key, fallback string) string {
	value, err := e.settings.Value(ctx, key)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func (e *Engine) intSetting(ctx context.Context, key string, fallback, minimum, maximum int) int {
	value, err := e.settings.Value(ctx, key)
	if err != nil {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return fallback
	}
	return parsed
}
