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
// later by ClaimSnapshotRuns / ClaimNextSnapshot.
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

// ClaimSnapshotRuns transitions all of a snapshot's pending runs to "running"
// (setting started_at), claiming the snapshot for the caller. Returns the
// number of rows transitioned; zero means another caller already claimed the
// snapshot (or it has no pending runs). With a single DB writer (maxOpenConns
// = 1) this is naturally atomic — no two callers run it concurrently.
func (d *DB) ClaimSnapshotRuns(ctx context.Context, snapshotID int64) (int64, error) {
	now := time.Now().UnixMicro()
	res, err := d.ExecContext(ctx, `
		UPDATE extractor_runs
		   SET status = 'running', started_at = ?
		 WHERE snapshot_id = ? AND status = 'pending'`, now, snapshotID)
	if err != nil {
		return 0, fmt.Errorf("meta.ClaimSnapshotRuns: update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("meta.ClaimSnapshotRuns: rows affected: %w", err)
	}
	return n, nil
}

// ClaimNextSnapshot finds a snapshot that has pending runs and no running runs
// (so it is not already being worked), transitions that snapshot's pending
// runs to "running", and returns its id. ok is false when no snapshot is
// waiting. This is the worker loop's claim; like ClaimSnapshotRuns it relies on
// the single-writer connection for atomicity.
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
		 WHERE snapshot_id = ? AND status = 'pending'`, now, snapshotID); err != nil {
		return 0, false, fmt.Errorf("meta.ClaimNextSnapshot: claim: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("meta.ClaimNextSnapshot: commit: %w", err)
	}
	return snapshotID, true, nil
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
