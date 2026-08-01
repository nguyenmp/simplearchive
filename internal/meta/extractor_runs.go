package meta

import (
	"context"
	"fmt"
)

// ExtractorRun is the in-memory representation of an extractor_runs row. It
// records the outcome of a single Step emitted by an extractor for a snapshot.
type ExtractorRun struct {
	ID         int64
	Timestamp  int64
	Extractor  string // Step.Name, e.g. "dom", "favicon", "headers"
	Status     string // "succeeded", "failed", "skipped"
	Output     string // output filename, or error text
	StartedAt  int64  // epoch microseconds
	FinishedAt int64  // epoch microseconds; 0 when NULL
	Error      string // failure cause; empty when NULL
}

// InsertRun records a single extractor run for the given snapshot timestamp.
// startedAt and finishedAt are epoch microseconds. A zero finishedAt is stored
// as NULL. An empty errMsg is stored as NULL.
func (d *DB) InsertRun(ctx context.Context, r ExtractorRun) (int64, error) {
	var finishedAt any
	if r.FinishedAt != 0 {
		finishedAt = r.FinishedAt
	}
	var errArg any
	if r.Error != "" {
		errArg = r.Error
	}
	res, err := d.ExecContext(ctx, `
		INSERT INTO extractor_runs (timestamp, extractor, status, output, started_at, finished_at, error)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Timestamp, r.Extractor, r.Status, r.Output, r.StartedAt, finishedAt, errArg)
	if err != nil {
		return 0, fmt.Errorf("meta.InsertRun: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("meta.InsertRun: last insert id: %w", err)
	}
	return id, nil
}

// ListRunsByTimestamp returns all extractor runs for the given snapshot,
// ordered by id ascending (the order they were recorded).
func (d *DB) ListRunsByTimestamp(ctx context.Context, ts int64) ([]ExtractorRun, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, timestamp, extractor, status, COALESCE(output, ''), started_at, COALESCE(finished_at, 0), COALESCE(error, '')
		FROM extractor_runs
		WHERE timestamp = ?
		ORDER BY id ASC`, ts)
	if err != nil {
		return nil, fmt.Errorf("meta.ListRunsByTimestamp: query: %w", err)
	}
	defer rows.Close()

	out := make([]ExtractorRun, 0)
	for rows.Next() {
		var r ExtractorRun
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Extractor, &r.Status, &r.Output, &r.StartedAt, &r.FinishedAt, &r.Error); err != nil {
			return nil, fmt.Errorf("meta.ListRunsByTimestamp: scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("meta.ListRunsByTimestamp: rows: %w", err)
	}
	return out, nil
}
