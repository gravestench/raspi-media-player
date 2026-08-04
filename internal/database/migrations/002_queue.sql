CREATE TABLE queue_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    revision INTEGER NOT NULL DEFAULT 0,
    playback_status TEXT NOT NULL DEFAULT 'idle',
    volume INTEGER NOT NULL DEFAULT 100 CHECK (volume BETWEEN 0 AND 100),
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO queue_state (singleton) VALUES (1);

CREATE TABLE queue_items (
    id TEXT PRIMARY KEY,
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    display_name TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL UNIQUE,
    added_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX queue_items_source_url_unique ON queue_items(source_url);
