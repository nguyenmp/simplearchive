-- extractor_runs records the outcome of each extractor step run for a snapshot.
-- One row per Step emitted by an extractor (e.g. "dom", "favicon", "headers").
-- It powers per-extractor status in the UI and is designed to be reusable by the
-- future M3.5 job_steps worker table.
CREATE TABLE extractor_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp   INTEGER NOT NULL,
    extractor   TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    output      TEXT,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    error       TEXT,
    FOREIGN KEY (timestamp) REFERENCES snapshots(timestamp)
);

CREATE INDEX idx_extractor_runs_timestamp ON extractor_runs(timestamp);
