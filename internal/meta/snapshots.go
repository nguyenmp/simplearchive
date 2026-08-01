package meta

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nguyenmp/simplearchive/internal/archive"
)

// CreateSnapshot inserts a new snapshot row in the "pending" state and returns
// the new snapshot's surrogate id. The caller already holds the timestamp it
// passed in (the ArchiveBox directory name); the id is the foreign key target
// used by extractor_runs.
func (d *DB) CreateSnapshot(ctx context.Context, url string, ts int64) (int64, error) {
	now := time.Now().UnixMicro()
	res, err := d.ExecContext(ctx, `
		INSERT INTO snapshots
		    (timestamp, url, title, status, is_archived, created_at, updated_at)
		VALUES (?, ?, NULL, 'pending', 0, ?, ?)`,
		ts, url, now, now)
	if err != nil {
		return 0, fmt.Errorf("meta.CreateSnapshot: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("meta.CreateSnapshot: last insert id: %w", err)
	}
	return id, nil
}

// execer is satisfied by both *sql.DB and *sql.Tx, letting the upsert SQL run
// either standalone or inside an import transaction.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// UpsertSnapshot inserts or refreshes a snapshot row from an on-disk index.json
// entry. It is the import path's counterpart to the Create/Update pair used by
// 'add': an existing row (matched by timestamp) has its url, title, is_archived,
// status, and updated_at refreshed, while created_at is preserved; a new row is
// inserted with created_at = updated_at = the snapshot's own timestamp. This
// makes 'simplearchive import' idempotent and safe to re-run.
func (d *DB) UpsertSnapshot(ctx context.Context, e archive.IndexEntry) error {
	if err := upsertSnapshot(ctx, d.DB, e); err != nil {
		return fmt.Errorf("meta.UpsertSnapshot: %w", err)
	}
	return nil
}

// UpsertSnapshotTx runs the same upsert as UpsertSnapshot against an in-flight
// transaction so the import path can batch many snapshots into one commit.
func UpsertSnapshotTx(ctx context.Context, tx *sql.Tx, e archive.IndexEntry) error {
	if err := upsertSnapshot(ctx, tx, e); err != nil {
		return fmt.Errorf("meta.UpsertSnapshotTx: %w", err)
	}
	return nil
}

func upsertSnapshot(ctx context.Context, q execer, e archive.IndexEntry) error {
	var titleArg any
	if e.Title != "" {
		titleArg = e.Title
	}
	isArchived := 0
	if e.IsArchived {
		isArchived = 1
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO snapshots (timestamp, url, title, status, is_archived, created_at, updated_at)
		VALUES (?, ?, ?, 'succeeded', ?, ?, ?)
		ON CONFLICT(timestamp) DO UPDATE SET
			url = excluded.url,
			title = excluded.title,
			is_archived = excluded.is_archived,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		e.Timestamp, e.URL, titleArg, isArchived, e.Timestamp, e.Timestamp)
	return err
}

// MarkSnapshotFailed sets a snapshot's status to 'failed' (leaving is_archived
// unchanged) and bumps updated_at. It is used when the primary DOM extractor
// fails so the snapshot is not left stuck in 'pending'.
func (d *DB) MarkSnapshotFailed(ctx context.Context, ts int64) error {
	now := time.Now().UnixMicro()
	res, err := d.ExecContext(ctx, `
		UPDATE snapshots
		   SET status = 'failed', updated_at = ?
		 WHERE timestamp = ?`, now, ts)
	if err != nil {
		return fmt.Errorf("meta.MarkSnapshotFailed: update: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("meta.MarkSnapshotFailed: rows affected: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("meta.MarkSnapshotFailed: no row for timestamp %d", ts)
	}
	return nil
}

// UpdateSnapshot marks a snapshot as successfully archived, recording its title.
// A null title is stored when title is the empty string.
func (d *DB) UpdateSnapshot(ctx context.Context, ts int64, title string) error {
	now := time.Now().UnixMicro()
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
