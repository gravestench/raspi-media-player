CREATE TABLE app_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL,
    username_key TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin INTEGER NOT NULL DEFAULT 0 CHECK (is_admin IN (0, 1)),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX users_is_admin ON users(is_admin);

CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BLOB NOT NULL UNIQUE,
    csrf_hash BLOB NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TEXT
);
CREATE INDEX sessions_user_id ON sessions(user_id);
CREATE INDEX sessions_token_hash ON sessions(token_hash);

CREATE TABLE queue_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    revision INTEGER NOT NULL DEFAULT 0,
    playback_status TEXT NOT NULL DEFAULT 'idle',
    volume INTEGER NOT NULL DEFAULT 100 CHECK (volume BETWEEN 0 AND 100),
    current_item_id TEXT,
    title TEXT NOT NULL DEFAULT '',
    position_seconds REAL NOT NULL DEFAULT 0,
    duration_seconds REAL NOT NULL DEFAULT 0,
    paused INTEGER NOT NULL DEFAULT 0,
    buffering INTEGER NOT NULL DEFAULT 0,
    playback_error TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO queue_state (singleton) VALUES (1);

CREATE TABLE queue_items (
    id TEXT PRIMARY KEY,
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL UNIQUE,
    added_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    submitter_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    playback_status TEXT NOT NULL DEFAULT 'queued',
    playback_error TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX queue_items_source_url_unique ON queue_items(source_url);

CREATE TABLE stations (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    stream_url TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX stations_household_name ON stations(lower(name)) WHERE owner_user_id IS NULL;
CREATE UNIQUE INDEX stations_owner_name ON stations(owner_user_id, lower(name)) WHERE owner_user_id IS NOT NULL;
INSERT INTO stations (id, owner_user_id, name, stream_url)
VALUES ('household-kfjc', NULL, 'KFJC 89.7 FM', 'https://netcast.kfjc.org/kfjc-128k-mp3');

CREATE TABLE favorites (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    station_id TEXT NOT NULL REFERENCES stations(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, station_id)
);

CREATE TABLE playlists (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (owner_user_id, name)
);

CREATE TABLE playlist_items (
    id TEXT PRIMARY KEY,
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    position INTEGER NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (playlist_id, position)
);

CREATE TABLE playback_history (
    id TEXT PRIMARY KEY,
    queue_item_id TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    submitter_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT,
    outcome TEXT NOT NULL DEFAULT 'playing',
    playback_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX playback_history_started_at ON playback_history(started_at DESC);
CREATE INDEX playback_history_queue_item ON playback_history(queue_item_id);

CREATE TABLE media_enrichments (
    cache_key TEXT PRIMARY KEY,
    artist TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    artist_url TEXT NOT NULL DEFAULT '',
    image_url TEXT NOT NULL DEFAULT '',
    image_source_url TEXT NOT NULL DEFAULT '',
    image_attribution TEXT NOT NULL DEFAULT '',
    biography TEXT NOT NULL DEFAULT '',
    genres_json TEXT NOT NULL DEFAULT '[]',
    related_artists_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL CHECK (status IN ('pending', 'ready', 'not_found', 'failed')),
    error_code TEXT NOT NULL DEFAULT '',
    fetched_at TEXT,
    expires_at TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX media_enrichments_expires_at ON media_enrichments(expires_at);

CREATE TABLE installation_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    completed_at TEXT
);
INSERT INTO installation_state (singleton, completed_at) VALUES (1, NULL);

CREATE TABLE application_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    secret INTEGER NOT NULL DEFAULT 0 CHECK (secret IN (0, 1)),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE auto_queue_user_turns (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_selected_at TEXT NOT NULL
);

CREATE TABLE track_likes (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, source_url, title)
);
CREATE INDEX track_likes_user_created_at ON track_likes(user_id, created_at DESC);
