-- v6: add ON DELETE CASCADE to foreign keys so deleting a snapshot
-- automatically removes its extractor_runs and step_outputs.
-- SQLite cannot ALTER TABLE to add CASCADE, so we rebuild both tables.

-- Rebuild extractor_runs -----------------------------------------------------
CREATE TABLE extractor_runs_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL,
    extractor   TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    started_at  INTEGER,
    finished_at INTEGER,
    error       TEXT,
    FOREIGN KEY (snapshot_id) REFERENCES snapshots(id) ON DELETE CASCADE
);

INSERT INTO extractor_runs_new (id, snapshot_id, extractor, status, started_at, finished_at, error)
SELECT id, snapshot_id, extractor, status, started_at, finished_at, error FROM extractor_runs;

DROP TABLE extractor_runs;
ALTER TABLE extractor_runs_new RENAME TO extractor_runs;

CREATE INDEX idx_extractor_runs_snapshot_id ON extractor_runs(snapshot_id);
CREATE INDEX idx_extractor_runs_status      ON extractor_runs(status);

-- Rebuild step_outputs -------------------------------------------------------
CREATE TABLE step_outputs_new (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id   INTEGER NOT NULL,
    name     TEXT    NOT NULL,
    filename TEXT,
    cmd      TEXT,
    status   TEXT    NOT NULL,
    start_ts INTEGER,
    end_ts   INTEGER,
    error    TEXT,
    FOREIGN KEY (run_id) REFERENCES extractor_runs(id) ON DELETE CASCADE
);

INSERT INTO step_outputs_new (id, run_id, name, filename, cmd, status, start_ts, end_ts, error)
SELECT id, run_id, name, filename, cmd, status, start_ts, end_ts, error FROM step_outputs;

DROP TABLE step_outputs;
ALTER TABLE step_outputs_new RENAME TO step_outputs;

CREATE INDEX idx_step_outputs_run_id ON step_outputs(run_id);
