package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Snapshot is the in-memory representation of a snapshots row. Title is the
// empty string when the column is NULL. ID is the surrogate primary key used
// by foreign keys; Timestamp is the ArchiveBox directory name / route key.
// Snapshot status is not stored — it is derived from the snapshot's
// extractor_runs: succeeded if any run succeeded, failed if any failed,
// pending/running if any are non-terminal, skipped otherwise. Imported
// snapshots with no extractor_runs default to succeeded.
type Snapshot struct {
	ID        int64
	Timestamp int64
	URL       string
	Title     string
	CreatedAt int64
	UpdatedAt int64
}

// ErrNotFound is returned by GetSnapshot when no row matches the timestamp.
var ErrNotFound = errors.New("snapshot not found")

// ListSnapshots returns up to limit snapshots ordered newest-first, starting at
// the given offset. The total row count (ignoring limit/offset) is returned
// alongside so callers can render pagination. A limit <= 0 or > maxLimit is
// clamped to maxLimit.
func (d *DB) ListSnapshots(ctx context.Context, limit, offset int) ([]Snapshot, int, error) {
	if limit <= 0 || limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	if err := d.db.QueryRowContext(ctx, "SELECT count(*) FROM snapshots").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("meta.ListSnapshots: count: %w", err)
	}

	rows, err := d.db.QueryContext(ctx, `
		SELECT id, timestamp, url, COALESCE(title, ''), created_at, updated_at
		FROM snapshots
		ORDER BY timestamp DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("meta.ListSnapshots: query: %w", err)
	}
	defer rows.Close()

	out := make([]Snapshot, 0, limit)
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.Timestamp, &s.URL, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("meta.ListSnapshots: scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("meta.ListSnapshots: rows: %w", err)
	}
	return out, total, nil
}

// GetSnapshotsByTimestamps returns the snapshots for the provided timestamps,
// ordered newest-first. Timestamps not present in the database are silently
// omitted.
func (d *DB) GetSnapshotsByTimestamps(ctx context.Context, timestamps []int64) ([]Snapshot, error) {
	if len(timestamps) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(timestamps))
	args := make([]any, len(timestamps))
	for i, ts := range timestamps {
		placeholders[i] = "?"
		args[i] = ts
	}

	query := fmt.Sprintf(`
		SELECT id, timestamp, url, COALESCE(title, ''), created_at, updated_at
		FROM snapshots
		WHERE timestamp IN (%s)
		ORDER BY timestamp DESC`, strings.Join(placeholders, ","))

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("meta.GetSnapshotsByTimestamps: query: %w", err)
	}
	defer rows.Close()

	out := make([]Snapshot, 0, len(timestamps))
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.Timestamp, &s.URL, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("meta.GetSnapshotsByTimestamps: scan: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("meta.GetSnapshotsByTimestamps: rows: %w", err)
	}
	return out, nil
}

// GetSnapshot returns the single snapshot identified by timestamp. It returns
// ErrNotFound when no row matches.
func (d *DB) GetSnapshot(ctx context.Context, timestamp int64) (Snapshot, error) {
	var s Snapshot
	err := d.db.QueryRowContext(ctx, `
		SELECT id, timestamp, url, COALESCE(title, ''), created_at, updated_at
		FROM snapshots
		WHERE timestamp = ?`, timestamp).Scan(
		&s.ID, &s.Timestamp, &s.URL, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, fmt.Errorf("meta.GetSnapshot: %w", err)
	}
	return s, nil
}

// GetSnapshotByID returns the single snapshot identified by its surrogate id. It
// returns ErrNotFound when no row matches. Used by the worker, which dispatches
// by id (the foreign key), not by the ArchiveBox timestamp.
func (d *DB) GetSnapshotByID(ctx context.Context, id int64) (Snapshot, error) {
	var s Snapshot
	err := d.db.QueryRowContext(ctx, `
		SELECT id, timestamp, url, COALESCE(title, ''), created_at, updated_at
		FROM snapshots
		WHERE id = ?`, id).Scan(
		&s.ID, &s.Timestamp, &s.URL, &s.Title, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, fmt.Errorf("meta.GetSnapshotByID: %w", err)
	}
	return s, nil
}

// DeleteSnapshot removes the snapshot identified by timestamp. ON DELETE
// CASCADE on the FKs automatically cleans up extractor_runs and step_outputs.
// It returns ErrNotFound when no row matches.
func (d *DB) DeleteSnapshot(ctx context.Context, timestamp int64) error {
	res, err := d.db.ExecContext(ctx, `DELETE FROM snapshots WHERE timestamp = ?`, timestamp)
	if err != nil {
		return fmt.Errorf("meta.DeleteSnapshot: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("meta.DeleteSnapshot: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
