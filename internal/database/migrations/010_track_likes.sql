CREATE TABLE track_likes (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, source_url)
);
CREATE INDEX track_likes_user_created_at ON track_likes(user_id, created_at DESC);
