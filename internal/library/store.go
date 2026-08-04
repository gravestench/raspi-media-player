package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("library item not found")
	ErrConflict  = errors.New("library item conflict")
	ErrForbidden = errors.New("library item forbidden")
)

type Station struct {
	ID          string `json:"id"`
	OwnerUserID string `json:"owner_user_id,omitempty"`
	Name        string `json:"name"`
	StreamURL   string `json:"stream_url"`
	Favorite    bool   `json:"favorite"`
	CreatedAt   string `json:"created_at"`
}
type PlaylistItem struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	SourceKind string `json:"source_kind"`
	SourceURL  string `json:"source_url"`
	Position   int    `json:"position"`
}
type Playlist struct {
	ID          string         `json:"id"`
	OwnerUserID string         `json:"owner_user_id"`
	Name        string         `json:"name"`
	Items       []PlaylistItem `json:"items"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}
type HistoryItem struct {
	ID              string `json:"id"`
	QueueItemID     string `json:"queue_item_id"`
	SourceKind      string `json:"source_kind"`
	SourceURL       string `json:"source_url"`
	Title           string `json:"title,omitempty"`
	SubmitterUserID string `json:"submitter_user_id,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
	Outcome         string `json:"outcome"`
	Error           string `json:"error,omitempty"`
}
type LikedTrack struct {
	SourceKind string `json:"source_kind"`
	SourceURL  string `json:"source_url"`
	Title      string `json:"title"`
	CreatedAt  string `json:"created_at"`
}
type SearchResults struct {
	Stations  []Station     `json:"stations"`
	Playlists []Playlist    `json:"playlists"`
	History   []HistoryItem `json:"history"`
}

type Store struct {
	db               *sql.DB
	historyRetention time.Duration
}

func NewStore(db *sql.DB, retention time.Duration) *Store {
	return &Store{db: db, historyRetention: retention}
}

func (s *Store) ListStations(ctx context.Context, userID, query string) ([]Station, error) {
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, COALESCE(s.owner_user_id, ''), s.name, s.stream_url, s.created_at, CASE WHEN f.user_id IS NULL THEN 0 ELSE 1 END FROM stations s LEFT JOIN favorites f ON f.station_id = s.id AND f.user_id = ? WHERE (s.owner_user_id IS NULL OR s.owner_user_id = ?) AND (lower(s.name) LIKE ? OR lower(s.stream_url) LIKE ?) ORDER BY s.owner_user_id IS NOT NULL, lower(s.name)`, userID, userID, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Station, 0)
	for rows.Next() {
		var value Station
		if err := rows.Scan(&value.ID, &value.OwnerUserID, &value.Name, &value.StreamURL, &value.CreatedAt, &value.Favorite); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) CreateStation(ctx context.Context, userID, name, streamURL string) (Station, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Station{}, errors.New("station name must be between 1 and 80 characters")
	}
	value := Station{ID: newID(), OwnerUserID: userID, Name: name, StreamURL: streamURL, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_, err := s.db.ExecContext(ctx, `INSERT INTO stations (id, owner_user_id, name, stream_url, created_at) VALUES (?, ?, ?, ?, ?)`, value.ID, value.OwnerUserID, value.Name, value.StreamURL, value.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Station{}, ErrConflict
		}
		return Station{}, err
	}
	return value, nil
}

func (s *Store) DeleteStation(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM stations WHERE id = ? AND owner_user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetFavorite(ctx context.Context, userID, stationID string, favorite bool) error {
	var visible int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stations WHERE id = ? AND (owner_user_id IS NULL OR owner_user_id = ?)`, stationID, userID).Scan(&visible); err != nil {
		return err
	}
	if visible == 0 {
		return ErrNotFound
	}
	if favorite {
		_, err := s.db.ExecContext(ctx, `INSERT INTO favorites (user_id, station_id) VALUES (?, ?) ON CONFLICT DO NOTHING`, userID, stationID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE user_id = ? AND station_id = ?`, userID, stationID)
	return err
}

func (s *Store) ListPlaylists(ctx context.Context, userID, query string) ([]Playlist, error) {
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT id, owner_user_id, name, created_at, updated_at FROM playlists WHERE owner_user_id = ? AND lower(name) LIKE ? ORDER BY lower(name)`, userID, pattern)
	if err != nil {
		return nil, err
	}
	result := make([]Playlist, 0)
	for rows.Next() {
		var value Playlist
		if err := rows.Scan(&value.ID, &value.OwnerUserID, &value.Name, &value.CreatedAt, &value.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		result = append(result, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range result {
		result[index].Items, err = s.playlistItems(ctx, result[index].ID)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Store) CreatePlaylist(ctx context.Context, userID, name string) (Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Playlist{}, errors.New("playlist name must be between 1 and 80 characters")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	value := Playlist{ID: newID(), OwnerUserID: userID, Name: name, Items: make([]PlaylistItem, 0), CreatedAt: now, UpdatedAt: now}
	_, err := s.db.ExecContext(ctx, `INSERT INTO playlists (id, owner_user_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, value.ID, userID, name, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Playlist{}, ErrConflict
		}
		return Playlist{}, err
	}
	return value, nil
}

func (s *Store) DeletePlaylist(ctx context.Context, userID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM playlists WHERE id = ? AND owner_user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddPlaylistItem(ctx context.Context, userID, playlistID, name, kind, sourceURL string) (PlaylistItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlaylistItem{}, err
	}
	defer tx.Rollback()
	var position int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(i.position), -1) + 1 FROM playlists p LEFT JOIN playlist_items i ON i.playlist_id = p.id WHERE p.id = ? AND p.owner_user_id = ?`, playlistID, userID).Scan(&position); err != nil {
		return PlaylistItem{}, err
	}
	var owns int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM playlists WHERE id = ? AND owner_user_id = ?`, playlistID, userID).Scan(&owns); err != nil {
		return PlaylistItem{}, err
	}
	if owns == 0 {
		return PlaylistItem{}, ErrNotFound
	}
	value := PlaylistItem{ID: newID(), Name: strings.TrimSpace(name), SourceKind: kind, SourceURL: sourceURL, Position: position}
	if _, err := tx.ExecContext(ctx, `INSERT INTO playlist_items (id, playlist_id, name, source_kind, source_url, position) VALUES (?, ?, ?, ?, ?, ?)`, value.ID, playlistID, value.Name, value.SourceKind, value.SourceURL, value.Position); err != nil {
		return PlaylistItem{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE playlists SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, playlistID)
	if err != nil {
		return PlaylistItem{}, err
	}
	return value, tx.Commit()
}

func (s *Store) RemovePlaylistItem(ctx context.Context, userID, playlistID, itemID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM playlist_items WHERE id = ? AND playlist_id = ? AND EXISTS (SELECT 1 FROM playlists WHERE id = ? AND owner_user_id = ?)`, itemID, playlistID, playlistID, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM playlist_items WHERE playlist_id = ? ORDER BY position`, playlistID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	_, _ = tx.ExecContext(ctx, `UPDATE playlist_items SET position = -position - 1 WHERE playlist_id = ?`, playlistID)
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE playlist_items SET position = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) playlistItems(ctx context.Context, playlistID string) ([]PlaylistItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, source_kind, source_url, position FROM playlist_items WHERE playlist_id = ? ORDER BY position`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PlaylistItem, 0)
	for rows.Next() {
		var value PlaylistItem
		if err := rows.Scan(&value.ID, &value.Name, &value.SourceKind, &value.SourceURL, &value.Position); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) RecordStarted(ctx context.Context, queueItemID, kind, sourceURL, submitterUserID string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, _ = tx.ExecContext(ctx, `UPDATE playback_history SET finished_at = ?, outcome = 'interrupted' WHERE queue_item_id = ? AND finished_at IS NULL`, now, queueItemID)
	id := newID()
	var submitter any
	if submitterUserID != "" {
		submitter = submitterUserID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO playback_history (id, queue_item_id, source_kind, source_url, submitter_user_id, started_at) VALUES (?, ?, ?, ?, ?, ?)`, id, queueItemID, kind, sourceURL, submitter, now); err != nil {
		return "", err
	}
	return id, tx.Commit()
}
func (s *Store) RecordFinished(ctx context.Context, queueItemID, title, outcome string, failure error) error {
	message := ""
	if failure != nil {
		message = failure.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE playback_history SET title = ?, finished_at = ?, outcome = ?, playback_error = ? WHERE id = (SELECT id FROM playback_history WHERE queue_item_id = ? AND finished_at IS NULL ORDER BY started_at DESC LIMIT 1)`, title, time.Now().UTC().Format(time.RFC3339Nano), outcome, message, queueItemID)
	if err != nil {
		return err
	}
	if s.historyRetention > 0 {
		cutoff := time.Now().Add(-s.historyRetention).UTC().Format(time.RFC3339Nano)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM playback_history WHERE started_at < ?`, cutoff)
	}
	return nil
}

func (s *Store) RecordTitleChanged(ctx context.Context, queueItemID, title string) (bool, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return false, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, existing, kind, sourceURL, startedAt string
	var submitter sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, title, source_kind, source_url, started_at, submitter_user_id FROM playback_history WHERE queue_item_id = ? AND finished_at IS NULL ORDER BY started_at DESC LIMIT 1`, queueItemID).Scan(&id, &existing, &kind, &sourceURL, &startedAt, &submitter)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if strings.EqualFold(strings.TrimSpace(existing), title) {
		return false, nil
	}
	if existing == "" {
		_, err = tx.ExecContext(ctx, `UPDATE playback_history SET title = ? WHERE id = ?`, title, id)
		return false, commitOrError(tx, err)
	}
	started, _ := time.Parse(time.RFC3339Nano, startedAt)
	if !started.IsZero() && time.Since(started) < 5*time.Second {
		return false, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE playback_history SET finished_at = ?, outcome = 'completed' WHERE id = ?`, now, id); err != nil {
		return false, err
	}
	newID := newID()
	if _, err := tx.ExecContext(ctx, `INSERT INTO playback_history (id, queue_item_id, source_kind, source_url, title, submitter_user_id, started_at, outcome) VALUES (?, ?, ?, ?, ?, ?, ?, 'playing')`, newID, queueItemID, kind, sourceURL, title, nullableString(submitter), now); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func nullableString(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}

func commitOrError(tx *sql.Tx, err error) error {
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) ListHistory(ctx context.Context, query string, limit int) ([]HistoryItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT id, queue_item_id, source_kind, source_url, title, COALESCE(submitter_user_id, ''), started_at, COALESCE(finished_at, ''), outcome, playback_error FROM playback_history WHERE lower(title) LIKE ? OR lower(source_url) LIKE ? ORDER BY started_at DESC LIMIT ?`, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]HistoryItem, 0)
	for rows.Next() {
		var value HistoryItem
		if err := rows.Scan(&value.ID, &value.QueueItemID, &value.SourceKind, &value.SourceURL, &value.Title, &value.SubmitterUserID, &value.StartedAt, &value.FinishedAt, &value.Outcome, &value.Error); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) ListUserHistory(ctx context.Context, userID string, limit int) ([]HistoryItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, queue_item_id, source_kind, source_url, title, COALESCE(submitter_user_id, ''), started_at, COALESCE(finished_at, ''), outcome, playback_error FROM playback_history WHERE submitter_user_id = ? ORDER BY started_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]HistoryItem, 0)
	for rows.Next() {
		var value HistoryItem
		if err := rows.Scan(&value.ID, &value.QueueItemID, &value.SourceKind, &value.SourceURL, &value.Title, &value.SubmitterUserID, &value.StartedAt, &value.FinishedAt, &value.Outcome, &value.Error); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) LikeTrack(ctx context.Context, userID, kind, sourceURL, title string) (LikedTrack, error) {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 500 {
		return LikedTrack{}, errors.New("track title must be between 1 and 500 characters")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `INSERT INTO track_likes (user_id, source_kind, source_url, title, created_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT(user_id, source_url, title) DO UPDATE SET source_kind = excluded.source_kind, created_at = excluded.created_at`, userID, kind, sourceURL, title, now)
	return LikedTrack{SourceKind: kind, SourceURL: sourceURL, Title: title, CreatedAt: now}, err
}

func (s *Store) ListLikedTracks(ctx context.Context, userID string, limit int) ([]LikedTrack, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_kind, source_url, title, created_at FROM track_likes WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]LikedTrack, 0)
	for rows.Next() {
		var value LikedTrack
		if err := rows.Scan(&value.SourceKind, &value.SourceURL, &value.Title, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) DeleteLikedTrack(ctx context.Context, userID, sourceURL, title string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM track_likes WHERE user_id = ? AND source_url = ? AND title = ?`, userID, sourceURL, title)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RemoveUserHistory removes a play from a user's profile while retaining the
// shared household playback record.
func (s *Store) RemoveUserHistory(ctx context.Context, userID, historyID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE playback_history SET submitter_user_id = NULL WHERE id = ? AND submitter_user_id = ?`, historyID, userID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Search(ctx context.Context, userID, query string) (SearchResults, error) {
	stations, err := s.ListStations(ctx, userID, query)
	if err != nil {
		return SearchResults{}, err
	}
	playlists := make([]Playlist, 0)
	if userID != "" {
		playlists, err = s.ListPlaylists(ctx, userID, query)
		if err != nil {
			return SearchResults{}, err
		}
	}
	history, err := s.ListHistory(ctx, query, 25)
	return SearchResults{Stations: stations, Playlists: playlists, History: history}, err
}

func newID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(value[:])
}
