-- v3: introduce a surrogate id primary key for snapshots, demoting timestamp
-- from PRIMARY KEY to a UNIQUE NOT NULL column. timestamp remains the
-- ArchiveBox directory name / route key / index.json key; id is the internal
-- identity that foreign keys will reference (extractor_runs.snapshot_id, added
-- in a later migration). SQLite cannot change a PRIMARY KEY in place, so this
-- is a table rebuild: copy all rows, drop the old table, rename.
--
-- The rebuild drops the parent table of the existing
-- extractor_runs(timestamp) foreign key, so migrate() runs this with FK
-- enforcement off and verifies PRAGMA foreign_key_check afterwards. The data
-- copy preserves every timestamp, so the FK stays valid.
CREATE TABLE snapshots_new (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp   INTEGER NOT NULL UNIQUE,
    url         TEXT    NOT NULL,
    title       TEXT,
    status      TEXT    NOT NULL DEFAULT 'pending',
    is_archived INTEGER NOT NULL DEFAULT 0,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

INSERT INTO snapshots_new (timestamp, url, title, status, is_archived, created_at, updated_at)
SELECT timestamp, url, title, status, is_archived, created_at, updated_at FROM snapshots;

DROP TABLE snapshots;
ALTER TABLE snapshots_new RENAME TO snapshots;
