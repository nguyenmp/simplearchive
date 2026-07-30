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
