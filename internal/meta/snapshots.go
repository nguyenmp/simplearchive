package meta

import (
	"context"
	"fmt"
	"time"
)

// CreateSnapshot inserts a new snapshot row in the "pending" state and returns
// the timestamp it was stored under.
func (d *DB) CreateSnapshot(ctx context.Context, url string, ts int64) (int64, error) {
	now := time.Now().UnixMilli()
	_, err := d.ExecContext(ctx, `
		INSERT INTO snapshots
		    (timestamp, url, title, status, is_archived, created_at, updated_at)
		VALUES (?, ?, NULL, 'pending', 0, ?, ?)`,
		ts, url, now, now)
	if err != nil {
		return 0, fmt.Errorf("meta.CreateSnapshot: insert: %w", err)
	}
	return ts, nil
}

// UpdateSnapshot marks a snapshot as successfully archived, recording its title.
// A null title is stored when title is the empty string.
func (d *DB) UpdateSnapshot(ctx context.Context, ts int64, title string) error {
	now := time.Now().UnixMilli()
	var titleArg any
	if title != "" {
		titleArg = title
	}
	res, err := d.ExecContext(ctx, `
		UPDATE snapshots
		   SET status = 'succeeded', is_archived = 1, title = ?, updated_at = ?
		 WHERE timestamp = ?`,
		titleArg, now, ts)
	if err != nil {
		return fmt.Errorf("meta.UpdateSnapshot: update: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("meta.UpdateSnapshot: rows affected: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("meta.UpdateSnapshot: no row for timestamp %d", ts)
	}
	return nil
}
