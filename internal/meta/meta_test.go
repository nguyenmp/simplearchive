package meta

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func userVersion(t *testing.T, db *DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	return v
}

func TestOpen_memory(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open(\"memory:\") = %v", err)
	}
	defer db.Close()

	if db.DB == nil {
		t.Fatal("opened DB has nil sql.DB")
	}
}

func TestOpen_fileAndReopen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")

	for i := 0; i < 2; i++ {
		db, err := Open(context.Background(), path)
		if err != nil {
			t.Fatalf("open %d: %v", i+1, err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close %d: %v", i+1, err)
		}
	}
}

func TestOpen_invalidPath(t *testing.T) {
	t.Parallel()
	// A path under a nonexistent directory cannot be created.
	_, err := Open(context.Background(), "/nonexistent/dir/meta.db")
	if err == nil {
		t.Fatal("expected error opening database under nonexistent directory")
	}
}

func TestClose_nilSafe(t *testing.T) {
	t.Parallel()
	var d *DB
	if err := d.Close(); err != nil {
		t.Fatalf("(*DB).Close on nil receiver = %v", err)
	}
}

func TestMigrate_freshCreatesSnapshotsTable(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if got := userVersion(t, db); got != 3 {
		t.Fatalf("user_version = %d, want 3", got)
	}
	if _, err := db.Exec("SELECT 1 FROM snapshots LIMIT 1"); err != nil {
		t.Fatalf("snapshots table not queryable after fresh open: %v", err)
	}
	if _, err := db.Exec("SELECT 1 FROM extractor_runs LIMIT 1"); err != nil {
		t.Fatalf("extractor_runs table not queryable after fresh open: %v", err)
	}
	// v3 added a surrogate id primary key to snapshots.
	var id int64
	if err := db.QueryRow("SELECT id FROM snapshots LIMIT 1").Scan(&id); err == nil {
		t.Errorf("fresh snapshots should be empty, but scanned id=%d", id)
	} else if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("query id from snapshots: %v", err)
	}
}

func TestMigrate_idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")

	if _, err := Open(context.Background(), path); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// Reopen must be a no-op: user_version stays at 2, no CREATE error.
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db.Close()

	if got := userVersion(t, db); got != 3 {
		t.Fatalf("user_version after reopen = %d, want 3", got)
	}
}

// TestMigrate_v3RebuildsSnapshotsWithSurrogateId seeds a v2 database (snapshots
// with timestamp as PRIMARY KEY plus an extractor_runs row referencing it),
// then opens via meta.Open so v3 rebuilds snapshots with a surrogate id PK. It
// verifies the row is preserved with a non-zero id, its timestamp intact, and
// no foreign-key violations remain.
func TestMigrate_v3RebuildsSnapshotsWithSurrogateId(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")
	ctx := context.Background()

	// Build a v2 database by hand: apply v1 + v2, seed a snapshot and a child
	// extractor_runs row, then pin user_version at 2.
	raw, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.ExecContext(ctx, schemaV1); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	if _, err := raw.ExecContext(ctx, schemaV2); err != nil {
		t.Fatalf("apply v2: %v", err)
	}
	const ts int64 = 1700000000000000
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO snapshots (timestamp, url, status, is_archived, created_at, updated_at)
		VALUES (?, 'https://example.com', 'succeeded', 1, 1, 1)`, ts); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO extractor_runs (timestamp, extractor, status, started_at, finished_at)
		VALUES (?, 'dom', 'succeeded', 1, 2)`, ts); err != nil {
		t.Fatalf("seed extractor_run: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		t.Fatalf("set user_version=2: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}

	// Open via meta.Open: migrates v2 -> v3 (rebuild snapshots with id PK).
	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if got := userVersion(t, db); got != 3 {
		t.Fatalf("user_version = %d, want 3", got)
	}

	var id int64
	var gotTS int64
	if err := db.QueryRowContext(ctx,
		"SELECT id, timestamp FROM snapshots WHERE timestamp = ?", ts).Scan(&id, &gotTS); err != nil {
		t.Fatalf("query migrated snapshot: %v", err)
	}
	if id == 0 {
		t.Errorf("id = 0, want a non-zero surrogate id")
	}
	if gotTS != ts {
		t.Errorf("timestamp = %d, want %d", gotTS, ts)
	}

	// The extractor_runs(timestamp) FK still resolves to a snapshots row.
	var runTS int64
	if err := db.QueryRowContext(ctx,
		"SELECT timestamp FROM extractor_runs WHERE timestamp = ?", ts).Scan(&runTS); err != nil {
		t.Fatalf("query extractor_run: %v", err)
	}
	if runTS != ts {
		t.Errorf("extractor_run timestamp = %d, want %d", runTS, ts)
	}

	// No dangling foreign keys should remain after the rebuild.
	rows, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	violations := 0
	for rows.Next() {
		violations++
	}
	rows.Close()
	if violations > 0 {
		t.Errorf("foreign_key_check reported %d violation(s), want 0", violations)
	}
}