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
