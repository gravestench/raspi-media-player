ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1));

-- Preserve access for upgraded installations by promoting the oldest account.
UPDATE users
SET is_admin = 1
WHERE id = (SELECT id FROM users ORDER BY created_at, id LIMIT 1);

CREATE TABLE installation_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    completed_at TEXT
);

INSERT INTO installation_state (singleton, completed_at)
SELECT 1, CASE WHEN EXISTS (SELECT 1 FROM users) THEN CURRENT_TIMESTAMP ELSE NULL END;

CREATE TABLE application_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    secret INTEGER NOT NULL DEFAULT 0 CHECK (secret IN (0, 1)),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX users_is_admin ON users(is_admin);
