package settings

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"sort"
	"strings"
)

type Definition struct {
	Key             string   `json:"key"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	Type            string   `json:"type"`
	Value           string   `json:"value,omitempty"`
	Options         []string `json:"options,omitempty"`
	Secret          bool     `json:"secret"`
	Configured      bool     `json:"configured,omitempty"`
	RestartRequired bool     `json:"restart_required"`
	ReadOnly        bool     `json:"read_only,omitempty"`
}

type Store struct {
	db          *sql.DB
	definitions map[string]Definition
	aead        cipher.AEAD
}

func NewStore(db *sql.DB, definitions []Definition, encryptionKey string) (*Store, error) {
	values := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		values[definition.Key] = definition
	}
	store := &Store{db: db, definitions: values}
	if strings.TrimSpace(encryptionKey) != "" {
		key := sha256.Sum256([]byte(encryptionKey))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return nil, err
		}
		store.aead, err = cipher.NewGCM(block)
		if err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *Store) List(ctx context.Context) ([]Definition, error) {
	values := make(map[string]string)
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM application_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	result := make([]Definition, 0, len(s.definitions))
	for key, definition := range s.definitions {
		if stored, ok := values[key]; ok {
			if definition.Secret {
				definition.Configured = stored != ""
				definition.Value = ""
			} else {
				definition.Value = stored
			}
		} else if definition.Secret {
			definition.Configured = definition.Value != ""
			definition.Value = ""
		}
		result = append(result, definition)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Category == result[j].Category {
			return result[i].Label < result[j].Label
		}
		return result[i].Category < result[j].Category
	})
	return result, rows.Err()
}

func (s *Store) Set(ctx context.Context, key, value, userID string) error {
	definition, ok := s.definitions[key]
	if !ok {
		return errors.New("unknown setting")
	}
	if definition.ReadOnly {
		return errors.New("setting is managed by the service configuration file")
	}
	stored := strings.TrimSpace(value)
	if definition.Secret && stored != "" {
		if s.aead == nil {
			return errors.New("secret encryption is not configured")
		}
		nonce := make([]byte, s.aead.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			return err
		}
		stored = base64.RawURLEncoding.EncodeToString(append(nonce, s.aead.Seal(nil, nonce, []byte(stored), []byte(key))...))
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO application_settings (key, value, secret, updated_at, updated_by_user_id)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, secret = excluded.secret, updated_at = CURRENT_TIMESTAMP, updated_by_user_id = excluded.updated_by_user_id`, key, stored, definition.Secret, userID)
	return err
}

func (s *Store) Value(ctx context.Context, key string) (string, error) {
	definition, ok := s.definitions[key]
	if !ok {
		return "", errors.New("unknown setting")
	}
	var stored string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM application_settings WHERE key = ?`, key).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return definition.Value, nil
	}
	if err != nil {
		return "", err
	}
	if !definition.Secret || stored == "" {
		return stored, nil
	}
	if s.aead == nil {
		return "", errors.New("secret encryption is not configured")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(stored)
	if err != nil || len(encoded) < s.aead.NonceSize() {
		return "", errors.New("stored secret is invalid")
	}
	nonce := encoded[:s.aead.NonceSize()]
	plain, err := s.aead.Open(nil, nonce, encoded[s.aead.NonceSize():], []byte(key))
	if err != nil {
		return "", errors.New("stored secret cannot be decrypted")
	}
	return string(plain), nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if _, ok := s.definitions[key]; !ok {
		return errors.New("unknown setting")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM application_settings WHERE key = ?`, key)
	return err
}
