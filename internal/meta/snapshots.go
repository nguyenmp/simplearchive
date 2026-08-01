package meta

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/nguyenmp/simplearchive/internal/archive"
)

// CreateSnapshot inserts a new snapshot row and returns the new snapshot's
// surrogate id. The caller already holds the timestamp it passed in (the
// ArchiveBox directory name); the id is the foreign key target used by
// extractor_runs. A snapshot has no stored status: its state is derived from
// its extractor_runs (see the Deferred milestone).
func (d *DB) CreateSnapshot(ctx context.Context, url string, ts int64) (int64, error) {
	now := time.Now().UnixMicro()
	res, err := d.ExecContext(ctx, `
		INSERT INTO snapshots (timestamp, url, title, created_at, updated_at)
		VALUES (?, ?, NULL, ?, ?)`,
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
// 'add': an existing row (matched by timestamp) has its url, title, and
// updated_at refreshed, while created_at is preserved; a new row is inserted
// with created_at = updated_at = the snapshot's own timestamp. This makes
// 'simplearchive import' idempotent and safe to re-run. A snapshot has no
// stored status; imported snapshots (no extractor_runs) derive to succeeded.
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
	_, err := q.ExecContext(ctx, `
		INSERT INTO snapshots (timestamp, url, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(timestamp) DO UPDATE SET
			url = excluded.url,
			title = excluded.title,
			updated_at = excluded.updated_at`,
		e.Timestamp, e.URL, titleArg, e.Timestamp, e.Timestamp)
	return err
}

// UpdateSnapshot records a snapshot's title after archiving, bumping updated_at.
// A null title is stored when title is the empty string. It returns an error if
// no row matches the timestamp. Snapshot status is not stored; it is derived
// from the snapshot's extractor_runs.
func (d *DB) UpdateSnapshot(ctx context.Context, ts int64, title string) error {
	now := time.Now().UnixMicro()
	var titleArg any
	if title != "" {
		titleArg = title
	}
	res, err := d.ExecContext(ctx, `
		UPDATE snapshots
		   SET title = ?, updated_at = ?
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
