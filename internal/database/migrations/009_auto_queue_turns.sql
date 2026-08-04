CREATE TABLE auto_queue_user_turns (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    last_selected_at TEXT NOT NULL
);

