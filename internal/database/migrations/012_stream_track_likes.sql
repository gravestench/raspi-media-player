DROP INDEX track_likes_user_created_at;
ALTER TABLE track_likes RENAME TO track_likes_by_source;

CREATE TABLE track_likes (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_kind TEXT NOT NULL,
    source_url TEXT NOT NULL,
    title TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, source_url, title)
);

INSERT INTO track_likes (user_id, source_kind, source_url, title, created_at)
SELECT user_id, source_kind, source_url, title, created_at FROM track_likes_by_source;

DROP TABLE track_likes_by_source;
CREATE INDEX track_likes_user_created_at ON track_likes(user_id, created_at DESC);
