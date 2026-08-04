package queue

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
	ErrDuplicate = errors.New("queue item already exists")
	ErrNotFound  = errors.New("queue item not found")
	ErrConflict  = errors.New("queue revision conflict")
	ErrFull      = errors.New("queue is full")
	ErrProtected = errors.New("queue item is protected")
)

type Item struct {
	ID          string         `json:"id"`
	Title       string         `json:"title,omitempty"`
	Source      Source         `json:"source"`
	Submitter   Submitter      `json:"submitter"`
	Position    int            `json:"position"`
	Status      string         `json:"status"`
	Error       string         `json:"error,omitempty"`
	AddedAt     string         `json:"added_at"`
	Default     bool           `json:"default"`
	RemovalVote *SkipVoteState `json:"removal_vote,omitempty"`
}

type Source struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type Submitter struct {
	UserID      string `json:"user_id,omitempty"`
	Username    string `json:"username,omitempty"`
	Kind        string `json:"kind"`
	DisplayName string `json:"display_name,omitempty"`
}

type PlaybackState struct {
	Status          string  `json:"status"`
	CurrentItemID   string  `json:"current_item_id,omitempty"`
	Title           string  `json:"title,omitempty"`
	PositionSeconds float64 `json:"position_seconds"`
	DurationSeconds float64 `json:"duration_seconds"`
	Paused          bool    `json:"paused"`
	Buffering       bool    `json:"buffering"`
	Volume          int     `json:"volume"`
	Error           string  `json:"error,omitempty"`
}

type Snapshot struct {
	Revision int64          `json:"revision"`
	Items    []Item         `json:"items"`
	Playback PlaybackState  `json:"playback"`
	SkipVote *SkipVoteState `json:"skip_vote,omitempty"`
}

type SkipVoteState struct {
	Enabled         bool   `json:"enabled"`
	CurrentItemID   string `json:"current_item_id,omitempty"`
	Votes           int    `json:"votes"`
	Required        int    `json:"required"`
	ActiveListeners int    `json:"active_listeners"`
	Voted           bool   `json:"voted"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

type Store struct{ db *sql.DB }
type UserSubmitter struct {
	ID       string
	Username string
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	var snapshot Snapshot
	if err := s.db.QueryRowContext(ctx, `SELECT revision, playback_status, volume, COALESCE(current_item_id, ''), title, position_seconds, duration_seconds, paused, buffering, playback_error FROM queue_state WHERE singleton = 1`).Scan(&snapshot.Revision, &snapshot.Playback.Status, &snapshot.Playback.Volume, &snapshot.Playback.CurrentItemID, &snapshot.Playback.Title, &snapshot.Playback.PositionSeconds, &snapshot.Playback.DurationSeconds, &snapshot.Playback.Paused, &snapshot.Playback.Buffering, &snapshot.Playback.Error); err != nil {
		return snapshot, fmt.Errorf("read queue state: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT q.id, q.title, q.source_kind, q.source_url, q.display_name, q.position, q.added_at, COALESCE(u.id, ''), COALESCE(u.username, ''), q.playback_status, q.playback_error, q.is_default FROM queue_items q LEFT JOIN users u ON u.id = q.submitter_user_id ORDER BY q.position`)
	if err != nil {
		return snapshot, fmt.Errorf("list queue: %w", err)
	}
	defer rows.Close()
	snapshot.Items = make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Title, &item.Source.Kind, &item.Source.URL, &item.Submitter.DisplayName, &item.Position, &item.AddedAt, &item.Submitter.UserID, &item.Submitter.Username, &item.Status, &item.Error, &item.Default); err != nil {
			return snapshot, fmt.Errorf("scan queue item: %w", err)
		}
		item.Submitter.Kind = "anonymous"
		if item.Submitter.UserID != "" {
			item.Submitter.Kind = "user"
			item.Submitter.DisplayName = ""
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, rows.Err()
}

func (s *Store) Add(ctx context.Context, sourceURL, displayName string, user *UserSubmitter, limit int) (Snapshot, Item, error) {
	return s.AddSource(ctx, "direct", sourceURL, displayName, user, limit)
}

func (s *Store) AddSource(ctx context.Context, sourceKind, sourceURL, displayName string, user *UserSubmitter, limit int) (Snapshot, Item, error) {
	return s.AddSourceTitled(ctx, sourceKind, sourceURL, "", displayName, user, limit)
}

func (s *Store) AddSourceTitled(ctx context.Context, sourceKind, sourceURL, title, displayName string, user *UserSubmitter, limit int) (Snapshot, Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, Item{}, err
	}
	defer tx.Rollback()
	var count, position int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), -1) + 1 FROM queue_items WHERE is_default = 0`).Scan(&count, &position); err != nil {
		return Snapshot{}, Item{}, err
	}
	if count >= limit {
		return Snapshot{}, Item{}, ErrFull
	}
	var fallbackPosition int
	if err := tx.QueryRowContext(ctx, `SELECT position FROM queue_items WHERE is_default = 1`).Scan(&fallbackPosition); err == nil {
		position = fallbackPosition
		if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = ? WHERE is_default = 1`, fallbackPosition+1); err != nil {
			return Snapshot{}, Item{}, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, Item{}, err
	}
	item := Item{ID: newID(), Title: title, Source: Source{Kind: sourceKind, URL: sourceURL}, Submitter: Submitter{Kind: "anonymous", DisplayName: displayName}, Position: position, Status: "queued", AddedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	var userID any
	if user != nil {
		item.Submitter = Submitter{Kind: "user", UserID: user.ID, Username: user.Username}
		userID = user.ID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO queue_items (id, title, source_kind, source_url, display_name, position, added_at, submitter_user_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, item.Title, item.Source.Kind, item.Source.URL, item.Submitter.DisplayName, item.Position, item.AddedAt, userID)
	if err != nil {
		if isUniqueError(err) {
			return Snapshot{}, Item{}, ErrDuplicate
		}
		return Snapshot{}, Item{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE queue_state SET revision = revision + 1, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`); err != nil {
		return Snapshot{}, Item{}, err
	}
	if err = tx.Commit(); err != nil {
		return Snapshot{}, Item{}, err
	}
	snapshot, err := s.Snapshot(ctx)
	return snapshot, item, err
}

func (s *Store) Remove(ctx context.Context, id string, expected int64) (Snapshot, error) {
	return s.mutate(ctx, expected, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE id = ? AND is_default = 0`, id)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			var exists int
			_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE id = ?`, id).Scan(&exists)
			if exists > 0 {
				return ErrProtected
			}
			return ErrNotFound
		}
		return compact(ctx, tx)
	})
}

func (s *Store) Clear(ctx context.Context, expected int64) (Snapshot, error) {
	return s.mutate(ctx, expected, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE is_default = 0`)
		if err == nil {
			err = compact(ctx, tx)
		}
		return err
	})
}

func (s *Store) Skip(ctx context.Context, expected int64) (Snapshot, error) {
	return s.mutate(ctx, expected, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE position = 0 AND is_default = 0`)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			var defaults int
			_ = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE position = 0 AND is_default = 1`).Scan(&defaults)
			if defaults > 0 {
				return ErrProtected
			}
			return ErrNotFound
		}
		return compact(ctx, tx)
	})
}

func (s *Store) Reorder(ctx context.Context, ids []string, expected int64) (Snapshot, error) {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			return Snapshot{}, ErrNotFound
		}
		seen[id] = struct{}{}
	}
	return s.mutate(ctx, expected, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items`).Scan(&count); err != nil {
			return err
		}
		if len(ids) != count {
			return ErrNotFound
		}
		for i, id := range ids {
			result, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = ? WHERE id = ?`, -(i + 1), id)
			if err != nil {
				return err
			}
			if affected, _ := result.RowsAffected(); affected != 1 {
				return ErrNotFound
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = -position - 1`)
		if err != nil {
			return err
		}
		return pinDefaultLast(ctx, tx)
	})
}

func (s *Store) EnsureDefault(ctx context.Context, sourceURL, title string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	sourceURL = strings.TrimSpace(sourceURL)
	title = strings.TrimSpace(title)
	changed := false
	if sourceURL == "" {
		result, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE is_default = 1`)
		if err != nil {
			return err
		}
		rows, _ := result.RowsAffected()
		changed = rows > 0
	} else {
		var id, existingURL string
		err := tx.QueryRowContext(ctx, `SELECT id, source_url FROM queue_items WHERE is_default = 1`).Scan(&id, &existingURL)
		if errors.Is(err, sql.ErrNoRows) {
			var duplicate int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE source_url = ?`, sourceURL).Scan(&duplicate); err != nil {
				return err
			}
			if duplicate == 0 {
				var position int
				if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM queue_items`).Scan(&position); err != nil {
					return err
				}
				_, err = tx.ExecContext(ctx, `INSERT INTO queue_items (id, title, source_kind, source_url, display_name, position, added_at, is_default) VALUES (?, ?, 'direct', ?, 'Default radio', ?, ?, 1)`, newID(), title, sourceURL, position, time.Now().UTC().Format(time.RFC3339Nano))
				if err != nil {
					return err
				}
				changed = true
			}
		} else if err != nil {
			return err
		} else {
			var duplicate int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_items WHERE source_url = ? AND id <> ?`, sourceURL, id).Scan(&duplicate); err != nil {
				return err
			}
			if duplicate > 0 {
				if _, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE id = ?`, id); err != nil {
					return err
				}
				changed = true
			} else {
				result, err := tx.ExecContext(ctx, `UPDATE queue_items SET title = CASE WHEN source_url <> ? THEN ? ELSE title END, source_url = ?, display_name = 'Default radio', playback_status = CASE WHEN playback_status = 'failed' THEN 'queued' ELSE playback_status END, playback_error = CASE WHEN playback_status = 'failed' THEN '' ELSE playback_error END WHERE id = ? AND (source_url <> ? OR playback_status = 'failed')`, sourceURL, title, sourceURL, id, sourceURL)
				if err != nil {
					return err
				}
				rows, _ := result.RowsAffected()
				changed = rows > 0 || existingURL != sourceURL
			}
		}
		if err := pinDefaultLast(ctx, tx); err != nil {
			return err
		}
	}
	if !changed {
		return tx.Commit()
	}
	if err := compact(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_state SET revision = revision + 1, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

func pinDefaultLast(ctx context.Context, tx *sql.Tx) error {
	var id string
	var position, maximum int
	if err := tx.QueryRowContext(ctx, `SELECT id, position FROM queue_items WHERE is_default = 1`).Scan(&id, &position); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM queue_items`).Scan(&maximum); err != nil {
		return err
	}
	if position == maximum {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = -1 WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = position - 1 WHERE position > ?`, position); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = ? WHERE id = ?`, maximum, id)
	return err
}

func (s *Store) SetCurrent(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET playback_status = 'queued', playback_error = '' WHERE playback_status = 'current'`); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE queue_items SET playback_status = 'current', playback_error = '' WHERE id = ? AND playback_status = 'queued'`, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_state SET current_item_id = ?, playback_status = 'loading', title = '', position_seconds = 0, duration_seconds = 0, paused = 0, buffering = 0, playback_error = '', updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdatePlayback(ctx context.Context, state PlaybackState) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE queue_state SET playback_status = ?, title = ?, position_seconds = ?, duration_seconds = ?, paused = ?, buffering = ?, volume = ?, playback_error = ?, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`, state.Status, state.Title, state.PositionSeconds, state.DurationSeconds, state.Paused, state.Buffering, state.Volume, state.Error); err != nil {
		return err
	}
	if state.Title != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE queue_items SET title = ? WHERE id = (SELECT current_item_id FROM queue_state WHERE singleton = 1) AND title <> ? AND (title = '' OR source_kind = 'direct')`, state.Title, state.Title); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetVolume(ctx context.Context, volume int) error {
	_, err := s.db.ExecContext(ctx, `UPDATE queue_state SET volume = ?, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`, volume)
	return err
}

func (s *Store) FinishCurrent(ctx context.Context, id string, failure error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if failure == nil {
		result, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE id = ? AND playback_status = 'current'`, id)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return ErrNotFound
		}
		if err := compact(ctx, tx); err != nil {
			return err
		}
	} else {
		result, err := tx.ExecContext(ctx, `UPDATE queue_items SET playback_status = 'failed', playback_error = ? WHERE id = ? AND playback_status = 'current'`, failure.Error(), id)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return ErrNotFound
		}
	}
	message := ""
	if failure != nil {
		message = failure.Error()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_state SET revision = revision + 1, current_item_id = NULL, playback_status = 'idle', title = '', position_seconds = 0, duration_seconds = 0, paused = 0, buffering = 0, playback_error = ?, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`, message); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RetryCurrent(ctx context.Context, id string, failure error) error {
	message := ""
	if failure != nil {
		message = failure.Error()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE queue_items SET playback_status = 'queued', playback_error = ? WHERE id = ? AND playback_status = 'current'`, message, id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_state SET current_item_id = NULL, playback_status = 'retrying', title = '', position_seconds = 0, duration_seconds = 0, paused = 0, buffering = 0, playback_error = ?, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`, message); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ResetPlayback(ctx context.Context, status, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE queue_state SET current_item_id = NULL, playback_status = ?, title = '', position_seconds = 0, duration_seconds = 0, paused = 0, buffering = 0, playback_error = ?, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`, status, message)
	return err
}

func (s *Store) ReconcilePlayback(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET playback_status = 'queued' WHERE playback_status = 'current'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_state SET current_item_id = NULL, playback_status = 'idle', title = '', position_seconds = 0, duration_seconds = 0, paused = 0, buffering = 0, playback_error = '', updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) mutate(ctx context.Context, expected int64, operation func(*sql.Tx) error) (Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM queue_state WHERE singleton = 1`).Scan(&revision); err != nil {
		return Snapshot{}, err
	}
	if revision != expected {
		return Snapshot{}, ErrConflict
	}
	if err := operation(tx); err != nil {
		return Snapshot{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE queue_state SET revision = revision + 1, updated_at = CURRENT_TIMESTAMP WHERE singleton = 1`); err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, err
	}
	return s.Snapshot(ctx)
}

func compact(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM queue_items ORDER BY position`)
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
	if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = -position - 1`); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE queue_items SET position = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return nil
}

func newID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}
func isUniqueError(err error) bool {
	return err != nil && (contains(err.Error(), "UNIQUE constraint failed"))
}
func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
