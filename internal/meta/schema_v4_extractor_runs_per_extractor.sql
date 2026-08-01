-- v4: reshape extractor_runs from per-output to per-extractor and add
-- step_outputs for the per-output detail. The legacy extractor_runs.extractor
-- column held Step.Name (dom, favicon, ...); the new column holds
-- Extractor.Name() (wget, wget-favicon, ...). Legacy rows are grouped into one
-- extractor_runs row per (snapshot, extractor) with step_outputs holding the
-- individual outputs. The foreign key moves from snapshots(timestamp) to
-- snapshots(id), introduced as the primary key in v3.
--
-- migrate() runs this with FK enforcement off and verifies
-- PRAGMA foreign_key_check afterwards; the data copy preserves referential
-- integrity by construction.

CREATE TABLE extractor_runs_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    snapshot_id INTEGER NOT NULL,
    extractor   TEXT    NOT NULL,
    status      TEXT    NOT NULL,
    started_at  INTEGER,
    finished_at INTEGER,
    error       TEXT,
    FOREIGN KEY (snapshot_id) REFERENCES snapshots(id)
);
CREATE INDEX idx_extractor_runs_snapshot_id ON extractor_runs_new(snapshot_id);
CREATE INDEX idx_extractor_runs_status      ON extractor_runs_new(status);

CREATE TABLE step_outputs (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id   INTEGER NOT NULL,
    name     TEXT    NOT NULL,
    filename TEXT,
    cmd      TEXT,
    status   TEXT    NOT NULL,
    start_ts INTEGER,
    end_ts   INTEGER,
    error    TEXT,
    FOREIGN KEY (run_id) REFERENCES extractor_runs(id)
);
CREATE INDEX idx_step_outputs_run_id ON step_outputs(run_id);

-- Map legacy Step.Name -> Extractor.Name() so legacy rows can be grouped.
CREATE TEMP TABLE _legacy_output_map (name TEXT PRIMARY KEY, extractor TEXT NOT NULL);
INSERT INTO _legacy_output_map (name, extractor) VALUES
    ('dom',             'wget'),
    ('favicon',         'wget-favicon'),
    ('headers',         'headers'),
    ('singlefile',      'obelisk'),
    ('screenshot',      'chromedp'),
    ('pdf',             'chromedp'),
    ('chromedp_dom',    'chromedp'),
    ('youtube_metadata','ytdlp'),
    ('transcript',      'ytdlp');

-- One extractor_runs row per (snapshot, extractor), aggregating its outputs.
INSERT INTO extractor_runs_new (snapshot_id, extractor, status, started_at, finished_at, error)
SELECT
    s.id,
    m.extractor,
    CASE
        WHEN SUM(CASE WHEN e.status = 'failed' THEN 1 ELSE 0 END) > 0 THEN 'failed'
        WHEN SUM(CASE WHEN e.status = 'succeeded' THEN 1 ELSE 0 END) > 0 THEN 'succeeded'
        ELSE 'skipped'
    END,
    MIN(e.started_at),
    MAX(e.finished_at),
    MIN(e.error)
FROM extractor_runs e
JOIN snapshots s ON s.timestamp = e.timestamp
JOIN _legacy_output_map m ON m.name = e.extractor
GROUP BY s.id, m.extractor;

-- One step_outputs row per legacy per-output row.
INSERT INTO step_outputs (run_id, name, filename, status, start_ts, end_ts, error)
SELECT
    er.id,
    e.extractor,
    e.output,
    e.status,
    e.started_at,
    e.finished_at,
    e.error
FROM extractor_runs e
JOIN snapshots s ON s.timestamp = e.timestamp
JOIN _legacy_output_map m ON m.name = e.extractor
JOIN extractor_runs_new er ON er.snapshot_id = s.id AND er.extractor = m.extractor;

DROP TABLE extractor_runs;
ALTER TABLE extractor_runs_new RENAME TO extractor_runs;
DROP TABLE _legacy_output_map;
