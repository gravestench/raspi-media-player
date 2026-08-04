package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

var (
	ErrDuplicate = errors.New("queue item already exists")
	ErrNotFound  = errors.New("queue item not found")
	ErrConflict  = errors.New("queue revision conflict")
	ErrFull      = errors.New("queue is full")
)

type Item struct {
	ID        string    `json:"id"`
	Source    Source    `json:"source"`
	Submitter Submitter `json:"submitter"`
	Position  int       `json:"position"`
	Status    string    `json:"status"`
	AddedAt   string    `json:"added_at"`
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
	Status string `json:"status"`
	Volume int    `json:"volume"`
}

type Snapshot struct {
	Revision int64         `json:"revision"`
	Items    []Item        `json:"items"`
	Playback PlaybackState `json:"playback"`
}

type Store struct{ db *sql.DB }
type UserSubmitter struct {
	ID       string
	Username string
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	var snapshot Snapshot
	if err := s.db.QueryRowContext(ctx, `SELECT revision, playback_status, volume FROM queue_state WHERE singleton = 1`).Scan(&snapshot.Revision, &snapshot.Playback.Status, &snapshot.Playback.Volume); err != nil {
		return snapshot, fmt.Errorf("read queue state: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT q.id, q.source_kind, q.source_url, q.display_name, q.position, q.added_at, COALESCE(u.id, ''), COALESCE(u.username, '') FROM queue_items q LEFT JOIN users u ON u.id = q.submitter_user_id ORDER BY q.position`)
	if err != nil {
		return snapshot, fmt.Errorf("list queue: %w", err)
	}
	defer rows.Close()
	snapshot.Items = make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.Source.Kind, &item.Source.URL, &item.Submitter.DisplayName, &item.Position, &item.AddedAt, &item.Submitter.UserID, &item.Submitter.Username); err != nil {
			return snapshot, fmt.Errorf("scan queue item: %w", err)
		}
		item.Submitter.Kind = "anonymous"
		if item.Submitter.UserID != "" {
			item.Submitter.Kind = "user"
			item.Submitter.DisplayName = ""
		}
		item.Status = "queued"
		if item.Position == 0 {
			item.Status = "current"
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, rows.Err()
}

func (s *Store) Add(ctx context.Context, sourceURL, displayName string, user *UserSubmitter, limit int) (Snapshot, Item, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, Item{}, err
	}
	defer tx.Rollback()
	var count, position int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(position), -1) + 1 FROM queue_items`).Scan(&count, &position); err != nil {
		return Snapshot{}, Item{}, err
	}
	if count >= limit {
		return Snapshot{}, Item{}, ErrFull
	}
	item := Item{ID: newID(), Source: Source{Kind: "direct", URL: sourceURL}, Submitter: Submitter{Kind: "anonymous", DisplayName: displayName}, Position: position, Status: "queued", AddedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	var userID any
	if user != nil {
		item.Submitter = Submitter{Kind: "user", UserID: user.ID, Username: user.Username}
		userID = user.ID
	}
	if position == 0 {
		item.Status = "current"
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO queue_items (id, source_kind, source_url, display_name, position, added_at, submitter_user_id) VALUES (?, ?, ?, ?, ?, ?, ?)`, item.ID, item.Source.Kind, item.Source.URL, item.Submitter.DisplayName, item.Position, item.AddedAt, userID)
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
		result, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return ErrNotFound
		}
		return compact(ctx, tx)
	})
}

func (s *Store) Clear(ctx context.Context, expected int64) (Snapshot, error) {
	return s.mutate(ctx, expected, func(tx *sql.Tx) error { _, err := tx.ExecContext(ctx, `DELETE FROM queue_items`); return err })
}

func (s *Store) Skip(ctx context.Context, expected int64) (Snapshot, error) {
	return s.mutate(ctx, expected, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM queue_items WHERE position = 0`)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
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
		return err
	})
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
