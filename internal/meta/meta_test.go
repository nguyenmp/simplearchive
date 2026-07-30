package meta

import (
	"context"
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

	if got := userVersion(t, db); got != 1 {
		t.Fatalf("user_version = %d, want 1", got)
	}
	if _, err := db.Exec("SELECT 1 FROM snapshots LIMIT 1"); err != nil {
		t.Fatalf("snapshots table not queryable after fresh open: %v", err)
	}
}

func TestMigrate_idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")

	if _, err := Open(context.Background(), path); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	// Reopen must be a no-op: user_version stays at 1, no CREATE error.
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db.Close()

	if got := userVersion(t, db); got != 1 {
		t.Fatalf("user_version after reopen = %d, want 1", got)
	}
}