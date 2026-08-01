-- v5: drop snapshot-level status and is_archived. Snapshot state is now
-- derived from its extractor_runs (see the Deferred milestone for the
-- derivation). This removes the redundant, stale-able snapshot-level status in
-- favor of per-step state as the single source of truth: a snapshot with no
-- extractor_runs rows is treated as succeeded (the imported-snapshot case),
-- and one with steps derives its state from the latest attempt per extractor.
ALTER TABLE snapshots DROP COLUMN status;
ALTER TABLE snapshots DROP COLUMN is_archived;
