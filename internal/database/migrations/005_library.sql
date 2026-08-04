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
