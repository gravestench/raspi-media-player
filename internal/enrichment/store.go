package enrichment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
