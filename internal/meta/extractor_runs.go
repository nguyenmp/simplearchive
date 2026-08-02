package meta

import (
	"context"
	"encoding/json"
	"fmt"
)

// ExtractorRun is the in-memory representation of an extractor_runs row: one
// extractor's run for a snapshot (e.g. "wget", "chromedp"). It is the unit of
// work and the unit of retry. A run produces zero or more StepOutputs. The
// latest attempt per (snapshot, extractor) is the current state; retries are
// new rows with higher ids.
type ExtractorRun struct {
	ID         int64
	SnapshotID int64
	Extractor  string // Extractor.Name(), e.g. "wget", "wget-favicon"
	Status     string // "pending" | "running" | "succeeded" | "failed" | "skipped"
	StartedAt  int64  // epoch microseconds; 0 when NULL
	FinishedAt int64  // epoch microseconds; 0 when NULL
	Error      string // failure cause; empty when NULL
	Outputs    []StepOutput
}

// StepOutput is one output an extractor run produced (e.g. wget -> "dom" +
// "favicon"; chromedp -> "screenshot" + "pdf" + "chromedp_dom"). Name is the
// ArchiveBox extractor/plugin key (Step.Name); Cmd is the shell argv recorded
// in index.json for debuggability and ArchiveBox reimport.
type StepOutput struct {
	ID       int64
	RunID    int64
	Name     string // Step.Name, e.g. "dom", "favicon"
	Filename string
	Cmd      []string
	Status   string
	StartTs  int64 // epoch microseconds; 0 when NULL
	EndTs    int64 // epoch microseconds; 0 when NULL
	Error    string
}

// InsertRun records a single extractor run for the given snapshot id.
// StartedAt/FinishedAt are epoch microseconds; a zero value is stored as NULL.
// An empty Error is stored as NULL. Outputs are not written here; use
// InsertStepOutput for each output. Returns the new run id.
func (d *DB) InsertRun(ctx context.Context, run ExtractorRun) (int64, error) {
	var startedAt, finishedAt any
	if run.StartedAt != 0 {
		startedAt = run.StartedAt
	}
	if run.FinishedAt != 0 {
		finishedAt = run.FinishedAt
	}
	var errArg any
	if run.Error != "" {
		errArg = run.Error
	}
	res, err := d.db.ExecContext(ctx, `
		INSERT INTO extractor_runs (snapshot_id, extractor, status, started_at, finished_at, error)
		VALUES (?, ?, ?, ?, ?, ?)`,
		run.SnapshotID, run.Extractor, run.Status, startedAt, finishedAt, errArg)
	if err != nil {
		return 0, fmt.Errorf("meta.InsertRun: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("meta.InsertRun: last insert id: %w", err)
	}
	return id, nil
}

// InsertStepOutput records one output of an extractor run. Cmd is stored as a
// JSON array; an empty Cmd is stored as NULL. A zero StartTs/EndTs is stored as
// NULL and an empty Error as NULL. Returns the new step_outputs id.
func (d *DB) InsertStepOutput(ctx context.Context, runID int64, out StepOutput) (int64, error) {
	var cmdArg any
	if len(out.Cmd) > 0 {
		b, err := json.Marshal(out.Cmd)
		if err != nil {
			return 0, fmt.Errorf("meta.InsertStepOutput: marshal cmd: %w", err)
		}
		cmdArg = string(b)
	}
	var startArg, endArg, errArg any
	if out.StartTs != 0 {
		startArg = out.StartTs
	}
	if out.EndTs != 0 {
		endArg = out.EndTs
	}
	if out.Error != "" {
		errArg = out.Error
	}
	res, err := d.db.ExecContext(ctx, `
		INSERT INTO step_outputs (run_id, name, filename, cmd, status, start_ts, end_ts, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, out.Name, out.Filename, cmdArg, out.Status, startArg, endArg, errArg)
	if err != nil {
		return 0, fmt.Errorf("meta.InsertStepOutput: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("meta.InsertStepOutput: last insert id: %w", err)
	}
	return id, nil
}

// ListRunsBySnapshot returns all extractor runs for the given snapshot id,
// ordered by id ascending (the order they were recorded), each with its
// StepOutputs populated (ordered by id).
func (d *DB) ListRunsBySnapshot(ctx context.Context, snapshotID int64) ([]ExtractorRun, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, snapshot_id, extractor, status,
		       COALESCE(started_at, 0), COALESCE(finished_at, 0), COALESCE(error, '')
		FROM extractor_runs
		WHERE snapshot_id = ?
		ORDER BY id ASC`, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("meta.ListRunsBySnapshot: query: %w", err)
	}
	runs := make([]ExtractorRun, 0)
	for rows.Next() {
		var run ExtractorRun
		if err := rows.Scan(&run.ID, &run.SnapshotID, &run.Extractor, &run.Status, &run.StartedAt, &run.FinishedAt, &run.Error); err != nil {
			rows.Close()
			return nil, fmt.Errorf("meta.ListRunsBySnapshot: scan: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("meta.ListRunsBySnapshot: rows: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("meta.ListRunsBySnapshot: close: %w", err)
	}
	if len(runs) == 0 {
		return runs, nil
	}

	ids := make([]any, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	stepOutputsByRun, err := d.queryStepOutputs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range runs {
		runs[i].Outputs = stepOutputsByRun[runs[i].ID]
	}
	return runs, nil
}

// queryStepOutputs fetches step_outputs for the given run ids and groups them
// by run id (ordered by id). The query uses an IN (...) list expanded from ids.
func (d *DB) queryStepOutputs(ctx context.Context, ids []any) (map[int64][]StepOutput, error) {
	query := `
		SELECT id, run_id, name, COALESCE(filename, ''), COALESCE(cmd, ''),
		       status, COALESCE(start_ts, 0), COALESCE(end_ts, 0), COALESCE(error, '')
		FROM step_outputs
		WHERE run_id IN (` + placeholders(len(ids)) + `)
		ORDER BY id ASC`
	rows, err := d.db.QueryContext(ctx, query, ids...)
	if err != nil {
		return nil, fmt.Errorf("meta.queryStepOutputs: query: %w", err)
	}
	defer rows.Close()

	out := make(map[int64][]StepOutput)
	for rows.Next() {
		var step StepOutput
		var cmd string
		if err := rows.Scan(&step.ID, &step.RunID, &step.Name, &step.Filename, &cmd, &step.Status, &step.StartTs, &step.EndTs, &step.Error); err != nil {
			return nil, fmt.Errorf("meta.queryStepOutputs: scan: %w", err)
		}
		if cmd != "" {
			if err := json.Unmarshal([]byte(cmd), &step.Cmd); err != nil {
				return nil, fmt.Errorf("meta.queryStepOutputs: unmarshal cmd: %w", err)
			}
		}
		out[step.RunID] = append(out[step.RunID], step)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("meta.queryStepOutputs: rows: %w", err)
	}
	return out, nil
}

// placeholders returns a "?, ?, ..." SQL placeholder list of n parameters.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	placeholders := make([]byte, 0, n*3-2)
	for i := 0; i < n; i++ {
		if i > 0 {
			placeholders = append(placeholders, ',', ' ')
		}
		placeholders = append(placeholders, '?')
	}
	return string(placeholders)
}
