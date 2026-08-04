package enrichment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrNotFound = errors.New("enrichment not found")

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Get(ctx context.Context, key string) (Result, error) {
	var value Result
	var genresJSON, relatedJSON string
	err := s.db.QueryRowContext(ctx, `SELECT cache_key, artist, title, provider, artist_url, image_url, image_source_url, image_attribution, biography, genres_json, related_artists_json, status, error_code, COALESCE(fetched_at, ''), expires_at FROM media_enrichments WHERE cache_key = ?`, key).Scan(&value.CacheKey, &value.Hint.Artist, &value.Hint.Title, &value.Provider, &value.ArtistURL, &value.Image.URL, &value.Image.SourceURL, &value.Image.Attribution, &value.Biography, &genresJSON, &relatedJSON, &value.Status, &value.ErrorCode, &value.FetchedAt, &value.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if err := json.Unmarshal([]byte(genresJSON), &value.Genres); err != nil {
		return Result{}, err
	}
	if err := json.Unmarshal([]byte(relatedJSON), &value.RelatedArtists); err != nil {
		return Result{}, err
	}
	return value, nil
}

func (s *Store) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Result{}, nil
	}
	if limit < 1 || limit > 100 {
		limit = 30
	}
	pattern := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT cache_key, artist, title, provider, artist_url, image_url, image_source_url, image_attribution, biography, genres_json, related_artists_json, status, error_code, COALESCE(fetched_at, ''), expires_at FROM media_enrichments WHERE status = 'ready' AND (lower(artist) LIKE ? OR lower(title) LIKE ? OR lower(genres_json) LIKE ? OR lower(related_artists_json) LIKE ?) ORDER BY updated_at DESC LIMIT ?`, pattern, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]Result, 0)
	for rows.Next() {
		var value Result
		var genresJSON, relatedJSON string
		if err := rows.Scan(&value.CacheKey, &value.Hint.Artist, &value.Hint.Title, &value.Provider, &value.ArtistURL, &value.Image.URL, &value.Image.SourceURL, &value.Image.Attribution, &value.Biography, &genresJSON, &relatedJSON, &value.Status, &value.ErrorCode, &value.FetchedAt, &value.ExpiresAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(genresJSON), &value.Genres); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(relatedJSON), &value.RelatedArtists); err != nil {
			return nil, err
		}
		results = append(results, value)
	}
	return results, rows.Err()
}

func (s *Store) Put(ctx context.Context, value Result) error {
	if value.CacheKey == "" {
		value.CacheKey = Key(value.Hint)
	}
	if value.Status == "" {
		value.Status = "pending"
	}
	if value.ExpiresAt == "" {
		value.ExpiresAt = time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano)
	}
	genres := value.Genres
	if genres == nil {
		genres = []string{}
	}
	related := value.RelatedArtists
	if related == nil {
		related = []ArtistSummary{}
	}
	genresJSON, err := json.Marshal(genres)
	if err != nil {
		return err
	}
	relatedJSON, err := json.Marshal(related)
	if err != nil {
		return err
	}
	var fetchedAt any
	if value.FetchedAt != "" {
		fetchedAt = value.FetchedAt
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO media_enrichments (cache_key,artist,title,provider,artist_url,image_url,image_source_url,image_attribution,biography,genres_json,related_artists_json,status,error_code,fetched_at,expires_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(cache_key) DO UPDATE SET artist=excluded.artist,title=excluded.title,provider=excluded.provider,artist_url=excluded.artist_url,image_url=excluded.image_url,image_source_url=excluded.image_source_url,image_attribution=excluded.image_attribution,biography=excluded.biography,genres_json=excluded.genres_json,related_artists_json=excluded.related_artists_json,status=excluded.status,error_code=excluded.error_code,fetched_at=excluded.fetched_at,expires_at=excluded.expires_at,updated_at=CURRENT_TIMESTAMP`, value.CacheKey, value.Hint.Artist, value.Hint.Title, value.Provider, value.ArtistURL, value.Image.URL, value.Image.SourceURL, value.Image.Attribution, value.Biography, string(genresJSON), string(relatedJSON), value.Status, value.ErrorCode, fetchedAt, value.ExpiresAt)
	return err
}
func (s *Store) Prune(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM media_enrichments WHERE expires_at < ?`, now.UTC().Format(time.RFC3339Nano))
	return err
}
