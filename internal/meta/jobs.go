package meta

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// InsertPendingRuns creates one "pending" extractor_runs row per extractor
// name for a snapshot, establishing the work the worker will drain. Each row
// has no started_at/finished_at yet (NULL). Runs are claimed (pending->running)
// later by ClaimNextSnapshot.
func (d *DB) InsertPendingRuns(ctx context.Context, snapshotID int64, extractors []string) error {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("meta.InsertPendingRuns: begin: %w", err)
	}
	defer tx.Rollback()
	for _, name := range extractors {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO extractor_runs (snapshot_id, extractor, status, started_at, finished_at, error)
			VALUES (?, ?, 'pending', NULL, NULL, NULL)`,
			snapshotID, name); err != nil {
			return fmt.Errorf("meta.InsertPendingRuns: insert %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("meta.InsertPendingRuns: commit: %w", err)
	}
	return nil
}

// ClaimNextSnapshot finds a snapshot that has pending runs and no running runs
// (so it is not already being worked), acquires the snapshot-level lock by
// transitioning that snapshot's oldest pending run to "running", and returns
// the snapshot id. ok is false when no snapshot is waiting.
//
// The lock is the presence of a "running" run: the NOT EXISTS clause above
// excludes any snapshot that already has a running run, so once a worker claims
// a snapshot (one run is running) no other claim will pick it up. Only the
// oldest pending run is flipped here; the worker starts each remaining pending
// run individually right before it executes (see ingest.runClaimedSnapshot), so
// "running" means "currently executing" rather than merely "claimed", and a
// crash between runs leaves the unstarted runs pending (reclaimable) rather than
// stranded in "running". This relies on the single-writer connection for
// atomicity.
//
// The same invariant also serializes index.json writes per snapshot: a
// snapshot can be rebuilt (see ingest.rebuildIndex) only while one of its runs
// is "running", so two workers never interleave writes to the same snapshot's
// index.json.
func (d *DB) ClaimNextSnapshot(ctx context.Context) (int64, bool, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("meta.ClaimNextSnapshot: begin: %w", err)
	}
	defer tx.Rollback()

	var snapshotID int64
	err = tx.QueryRowContext(ctx, `
		SELECT er.snapshot_id
		FROM extractor_runs er
		WHERE er.status = 'pending'
		  AND NOT EXISTS (
			SELECT 1 FROM extractor_runs r2
			WHERE r2.snapshot_id = er.snapshot_id AND r2.status = 'running'
		  )
		GROUP BY er.snapshot_id
		ORDER BY MIN(er.id)
		LIMIT 1`).Scan(&snapshotID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("meta.ClaimNextSnapshot: query: %w", err)
	}

	now := time.Now().UnixMicro()
	if _, err := tx.ExecContext(ctx, `
		UPDATE extractor_runs
		   SET status = 'running', started_at = ?
		 WHERE id = (
			SELECT id FROM extractor_runs
			WHERE snapshot_id = ? AND status = 'pending'
			ORDER BY id LIMIT 1
		 )`, now, snapshotID); err != nil {
		return 0, false, fmt.Errorf("meta.ClaimNextSnapshot: claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("meta.ClaimNextSnapshot: commit: %w", err)
	}
	return snapshotID, true, nil
}

// StartRun transitions a single pending run to "running" and stamps its
// started_at, right before the worker executes it. It is a no-op-style guard:
// only a run still "pending" is flipped (rows affected == 1), so a run already
// claimed by ClaimNextSnapshot (already "running") or already terminal is left
// untouched and the caller proceeds. This is what makes "running" mean
// "currently executing" rather than "claimed by the snapshot lock".
func (d *DB) StartRun(ctx context.Context, runID int64) error {
	now := time.Now().UnixMicro()
	res, err := d.ExecContext(ctx, `
		UPDATE extractor_runs
		   SET status = 'running', started_at = ?
		 WHERE id = ? AND status = 'pending'`, now, runID)
	if err != nil {
		return fmt.Errorf("meta.StartRun: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("meta.StartRun: rows affected: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("meta.StartRun: run %d not pending (rows affected %d)", runID, n)
	}
	return nil
}

// FinishRun marks a run terminal, recording its finished_at and (optionally)
// an error. A zero finishedAt is stored as NULL; an empty errMsg as NULL.
func (d *DB) FinishRun(ctx context.Context, runID int64, status string, finishedAt int64, errMsg string) error {
	var finArg any
	if finishedAt != 0 {
		finArg = finishedAt
	}
	var errArg any
	if errMsg != "" {
		errArg = errMsg
	}
	res, err := d.ExecContext(ctx, `
		UPDATE extractor_runs
		   SET status = ?, finished_at = ?, error = ?
		 WHERE id = ?`, status, finArg, errArg, runID)
	if err != nil {
		return fmt.Errorf("meta.FinishRun: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("meta.FinishRun: rows affected: %w", err)
	}
	if n != 1 {
		return fmt.Errorf("meta.FinishRun: no row for run %d", runID)
	}
	return nil
}

// DeleteStepOutputs removes all step_outputs for a run. It is used before
// re-recording a run's outputs so a re-run (e.g. after a crash left a run
// "running") does not accumulate duplicate output rows.
func (d *DB) DeleteStepOutputs(ctx context.Context, runID int64) error {
	if _, err := d.ExecContext(ctx, `DELETE FROM step_outputs WHERE run_id = ?`, runID); err != nil {
		return fmt.Errorf("meta.DeleteStepOutputs: delete: %w", err)
	}
	return nil
}
