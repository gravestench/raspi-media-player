package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrUsernameTaken   = errors.New("username taken")
	ErrInvalidSession  = errors.New("invalid session")
	ErrSessionNotFound = errors.New("session not found")
)

type User struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
}
type Session struct {
	ID         string `json:"id"`
	User       User   `json:"user"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	LastSeenAt string `json:"last_seen_at"`
}
type IssuedSession struct {
	Session   Session
	Token     string
	CSRFToken string
}

type Store struct {
	db              *sql.DB
	passwordParams  PasswordParams
	sessionLifetime time.Duration
}

func NewStore(db *sql.DB, params PasswordParams, lifetime time.Duration) *Store {
	return &Store{db: db, passwordParams: params, sessionLifetime: lifetime}
}

func NormalizeUsername(value string) (string, string, error) {
	username := strings.TrimSpace(value)
	if len(username) < 2 || len(username) > 32 {
		return "", "", errors.New("username must be between 2 and 32 characters")
	}
	for _, char := range username {
		if !(char >= 'a' && char <= 'z') && !(char >= 'A' && char <= 'Z') && !(char >= '0' && char <= '9') && char != '_' && char != '-' {
			return "", "", errors.New("username may contain only letters, numbers, hyphens, and underscores")
		}
	}
	return username, strings.ToLower(username), nil
}

func ValidatePassword(value string) error {
	if len(value) < 8 || len(value) > 256 {
		return errors.New("password must be between 8 and 256 characters")
	}
	return nil
}

func (s *Store) FindUser(ctx context.Context, username string) (User, string, error) {
	_, key, err := NormalizeUsername(username)
	if err != nil {
		return User{}, "", err
	}
	var user User
	var hash string
	err = s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, created_at FROM users WHERE username_key = ?`, key).Scan(&user.ID, &user.Username, &hash, &user.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", err
	}
	return user, hash, nil
}

func (s *Store) CreateUserAndSession(ctx context.Context, username, password string) (IssuedSession, error) {
	display, key, err := NormalizeUsername(username)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return IssuedSession{}, err
	}
	hash, err := HashPassword(password, s.passwordParams)
	if err != nil {
		return IssuedSession{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssuedSession{}, err
	}
	defer tx.Rollback()
	user := User{ID: randomToken(16), Username: display, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users (id, username, username_key, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`, user.ID, user.Username, key, hash, user.CreatedAt); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return IssuedSession{}, ErrUsernameTaken
		}
		return IssuedSession{}, err
	}
	issued, err := createSession(ctx, tx, user, s.sessionLifetime)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return IssuedSession{}, err
	}
	return issued, nil
}

func (s *Store) CreateSession(ctx context.Context, user User) (IssuedSession, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return IssuedSession{}, err
	}
	defer tx.Rollback()
	issued, err := createSession(ctx, tx, user, s.sessionLifetime)
	if err != nil {
		return IssuedSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return IssuedSession{}, err
	}
	return issued, nil
}

func createSession(ctx context.Context, tx *sql.Tx, user User, lifetime time.Duration) (IssuedSession, error) {
	token, csrf := randomToken(32), randomToken(32)
	now := time.Now().UTC()
	session := Session{ID: randomToken(16), User: user, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(lifetime).Format(time.RFC3339Nano), LastSeenAt: now.Format(time.RFC3339Nano)}
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, user_id, token_hash, csrf_hash, created_at, expires_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, session.ID, user.ID, digest(token), digest(csrf), session.CreatedAt, session.ExpiresAt, session.LastSeenAt)
	if err != nil {
		return IssuedSession{}, err
	}
	return IssuedSession{Session: session, Token: token, CSRFToken: csrf}, nil
}

func (s *Store) ResolveSession(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrInvalidSession
	}
	var session Session
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT s.id, u.id, u.username, u.created_at, s.created_at, s.expires_at, s.last_seen_at, s.revoked_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ?`, digest(token)).Scan(&session.ID, &session.User.ID, &session.User.Username, &session.User.CreatedAt, &session.CreatedAt, &session.ExpiresAt, &session.LastSeenAt, &revoked)
	if errors.Is(err, sql.ErrNoRows) || revoked.Valid {
		return Session{}, ErrInvalidSession
	}
	if err != nil {
		return Session{}, err
	}
	expires, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	if err != nil || time.Now().After(expires) {
		return Session{}, ErrInvalidSession
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = CURRENT_TIMESTAMP WHERE id = ?`, session.ID)
	return session, nil
}

func (s *Store) VerifyCSRF(ctx context.Context, sessionID, csrf string) bool {
	if csrf == "" {
		return false
	}
	var stored []byte
	if err := s.db.QueryRowContext(ctx, `SELECT csrf_hash FROM sessions WHERE id = ? AND revoked_at IS NULL`, sessionID).Scan(&stored); err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(stored, digest(csrf)) == 1
}

func (s *Store) Revoke(ctx context.Context, userID, sessionID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ? AND revoked_at IS NULL`, sessionID, userID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (s *Store) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, expires_at, last_seen_at FROM sessions WHERE user_id = ? AND revoked_at IS NULL AND datetime(expires_at) > CURRENT_TIMESTAMP ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Session, 0)
	for rows.Next() {
		var value Session
		if err := rows.Scan(&value.ID, &value.CreatedAt, &value.ExpiresAt, &value.LastSeenAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func digest(value string) []byte { result := sha256.Sum256([]byte(value)); return result[:] }
func randomToken(size int) string {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
