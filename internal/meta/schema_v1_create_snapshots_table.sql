CREATE TABLE snapshots (
    timestamp   INTEGER PRIMARY KEY,
    url         TEXT    NOT NULL,
    title       TEXT,
    status      TEXT    NOT NULL DEFAULT 'pending',
    is_archived INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);