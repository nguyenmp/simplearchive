package meta

import (
	"context"
	"fmt"
	"time"

	"github.com/nguyenmp/simplearchive/internal/archive"
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

// UpsertSnapshot inserts or refreshes a snapshot row from an on-disk index.json
// entry. It is the import path's counterpart to the Create/Update pair used by
// 'add': an existing row (matched by timestamp) has its url, title, is_archived,
// status, and updated_at refreshed, while created_at is preserved; a new row is
// inserted with created_at = updated_at = the snapshot's own timestamp. This
// makes 'simplearchive import' idempotent and safe to re-run.
func (d *DB) UpsertSnapshot(ctx context.Context, e archive.IndexEntry) error {
	var titleArg any
	if e.Title != "" {
		titleArg = e.Title
	}
	isArchived := 0
	if e.IsArchived {
		isArchived = 1
	}
	_, err := d.ExecContext(ctx, `
		INSERT INTO snapshots (timestamp, url, title, status, is_archived, created_at, updated_at)
		VALUES (?, ?, ?, 'succeeded', ?, ?, ?)
		ON CONFLICT(timestamp) DO UPDATE SET
			url = excluded.url,
			title = excluded.title,
			is_archived = excluded.is_archived,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		e.Timestamp, e.URL, titleArg, isArchived, e.Timestamp, e.Timestamp)
	if err != nil {
		return fmt.Errorf("meta.UpsertSnapshot: upsert: %w", err)
	}
	return nil
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
