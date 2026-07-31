package meta

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Snapshot is the in-memory representation of a snapshots row. Title is the
// empty string when the column is NULL.
type Snapshot struct {
	Timestamp  int64
	URL        string
	Title      string
	Status     string
	IsArchived bool
	CreatedAt  int64
	UpdatedAt  int64
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
	if err := d.QueryRowContext(ctx, "SELECT count(*) FROM snapshots").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("meta.ListSnapshots: count: %w", err)
	}

	rows, err := d.QueryContext(ctx, `
		SELECT timestamp, url, COALESCE(title, ''), status, is_archived, created_at, updated_at
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
		var isArchived int
		if err := rows.Scan(&s.Timestamp, &s.URL, &s.Title, &s.Status, &isArchived, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("meta.ListSnapshots: scan: %w", err)
		}
		s.IsArchived = isArchived != 0
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("meta.ListSnapshots: rows: %w", err)
	}
	return out, total, nil
}

// GetSnapshot returns the single snapshot identified by timestamp. It returns
// ErrNotFound when no row matches.
func (d *DB) GetSnapshot(ctx context.Context, ts int64) (Snapshot, error) {
	var s Snapshot
	var isArchived int
	err := d.QueryRowContext(ctx, `
		SELECT timestamp, url, COALESCE(title, ''), status, is_archived, created_at, updated_at
		FROM snapshots
		WHERE timestamp = ?`, ts).Scan(
		&s.Timestamp, &s.URL, &s.Title, &s.Status, &isArchived, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Snapshot{}, ErrNotFound
		}
		return Snapshot{}, fmt.Errorf("meta.GetSnapshot: %w", err)
	}
	s.IsArchived = isArchived != 0
	return s, nil
}
