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
		if item.Status == "queued" {
			queued++
		}
	}
	needed := depth - queued
	if needed <= 0 {
		return 0, nil
	}
	preferences, err := e.preferences(ctx, time.Now().Add(-time.Duration(activeSeconds)*time.Second))
	if err != nil || len(preferences) == 0 {
		return 0, err
	}
	limit := e.intSetting(ctx, "queue_limit", e.queueLimit, 1, 10000)
	added := 0
	for attempts := 0; added < needed && attempts < needed*6; attempts++ {
		preference := e.pick(preferences)
		query := preference.Name
		if preference.Kind == "genre" {
			query += " music"
			if key, keyErr := e.settings.Value(ctx, "lastfm_api_key"); keyErr == nil && key != "" {
				discoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				discovery, discoverErr := enrichment.NewLastFMProvider(key, nil).DiscoverTag(discoveryCtx, preference.Name, 50)
				cancel()
				if discoverErr == nil && len(discovery.Tracks) > 0 {
					track := discovery.Tracks[e.randomIndex(len(discovery.Tracks))]
					query = track.Artist + " " + track.Name
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
		_, _, addErr := e.queue.AddSource(ctx, "youtube", result.URL, "Auto-queue", nil, limit)
		if addErr == nil {
			added++
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

func (e *Engine) preferences(ctx context.Context, activeSince time.Time) ([]preference, error) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT h.submitter_user_id, h.title
		FROM playback_history h
		JOIN (
			SELECT DISTINCT user_id FROM sessions
			WHERE revoked_at IS NULL AND datetime(expires_at) > CURRENT_TIMESTAMP AND datetime(last_seen_at) >= datetime(?)
		) active ON active.user_id = h.submitter_user_id
		WHERE h.title <> ''
		ORDER BY h.started_at DESC
		LIMIT 1000`, activeSince.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type play struct{ userID, artist string }
	plays := make([]play, 0, 200)
	for rows.Next() {
		var userID, title string
		if err := rows.Scan(&userID, &title); err != nil {
			return nil, err
		}
		artist := strings.TrimSpace(enrichment.ParseTitle(title).Artist)
		if artist != "" {
			plays = append(plays, play{userID: userID, artist: artist})
		}
	}
	if err := rows.Err(); err != nil || len(plays) == 0 {
		return nil, err
	}
	genresByArtist, err := e.genresByArtist(ctx)
	if err != nil {
		return nil, err
	}
	artistCounts := map[string]map[string]int{}
	genreCounts := map[string]map[string]int{}
	labels := map[string]string{}
	for _, item := range plays {
		if artistCounts[item.userID] == nil {
			artistCounts[item.userID] = map[string]int{}
			genreCounts[item.userID] = map[string]int{}
		}
		key := strings.ToLower(item.artist)
		labels[key] = item.artist
		artistCounts[item.userID][key]++
		for _, genre := range genresByArtist[key] {
			genreKey := strings.ToLower(genre)
			labels["genre:"+genreKey] = genre
			genreCounts[item.userID][genreKey]++
		}
	}
	merged := map[string]int{}
	for userID, counts := range artistCounts {
		mergeNormalized(merged, "artist:", counts)
		mergeNormalized(merged, "genre:", genreCounts[userID])
	}
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
