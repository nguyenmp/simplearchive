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

	lv := latestVersion()
	if got := userVersion(t, db); got != lv {
		t.Fatalf("user_version = %d, want %d", got, lv)
	}
	if _, err := db.Exec("SELECT 1 FROM snapshots LIMIT 1"); err != nil {
		t.Fatalf("snapshots table not queryable after fresh open: %v", err)
	}
	if _, err := db.Exec("SELECT 1 FROM extractor_runs LIMIT 1"); err != nil {
		t.Fatalf("extractor_runs table not queryable after fresh open: %v", err)
	}
	if _, err := db.Exec("SELECT 1 FROM step_outputs LIMIT 1"); err != nil {
		t.Fatalf("step_outputs table not queryable after fresh open: %v", err)
	}
	// v6 added ON DELETE CASCADE to FKs, v5 dropped snapshot-level status and is_archived.
	if _, err := db.Exec("SELECT status FROM snapshots LIMIT 1"); err == nil {
		t.Errorf("snapshots.status column should have been dropped")
	}
	if _, err := db.Exec("SELECT is_archived FROM snapshots LIMIT 1"); err == nil {
		t.Errorf("snapshots.is_archived column should have been dropped")
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
	// Reopen must be a no-op: user_version stays at the latest, no CREATE error.
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db.Close()

	lv := latestVersion()
	if got := userVersion(t, db); got != lv {
		t.Fatalf("user_version after reopen = %d, want %d", got, lv)
	}
}

// seedV2DB builds a v2 database at path with one snapshot (timestamp ts) and
// one per-output extractor_runs row (extractor=outName, e.g. "dom") referencing
// it, then pins user_version at 2. It is the shared fixture for the v2->latest
// migration tests.
func seedV2DB(t *testing.T, path string, ts int64, outName string) {
	t.Helper()
	ctx := context.Background()
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
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO snapshots (timestamp, url, status, is_archived, created_at, updated_at)
		VALUES (?, 'https://example.com', 'succeeded', 1, 1, 1)`, ts); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `
		INSERT INTO extractor_runs (timestamp, extractor, status, output, started_at, finished_at)
		VALUES (?, ?, 'succeeded', 'output.html', 1, 2)`, ts, outName); err != nil {
		t.Fatalf("seed extractor_run: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "PRAGMA user_version = 2"); err != nil {
		t.Fatalf("set user_version=2: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw close: %v", err)
	}
}

func assertNoFKViolations(t *testing.T, db *DB) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), "PRAGMA foreign_key_check")
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

// TestMigrate_v3RebuildsSnapshotsWithSurrogateId seeds a v2 database and opens
// via meta.Open so v3 rebuilds snapshots with a surrogate id PK. It verifies
// the row is preserved with a non-zero id and its timestamp intact. (Opening
// also applies v4, but this test is scoped to the v3 snapshots concern.)
func TestMigrate_v3RebuildsSnapshotsWithSurrogateId(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")
	ctx := context.Background()
	const ts int64 = 1700000000000000
	seedV2DB(t, path, ts, "dom")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	lv := latestVersion()
	if got := userVersion(t, db); got != lv {
		t.Fatalf("user_version = %d, want %d", got, lv)
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
	assertNoFKViolations(t, db)
}

// TestMigrate_v4ReshapesExtractorRunsToPerExtractor seeds a v2 database with a
// per-output extractor_runs row (extractor="dom") and opens via meta.Open so
// v4 reshapes extractor_runs to per-extractor and adds step_outputs. It
// verifies the legacy "dom" output is grouped under a "wget" extractor run
// (FK snapshot_id) with a step_outputs row carrying the original output name.
func TestMigrate_v4ReshapesExtractorRunsToPerExtractor(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.db")
	ctx := context.Background()
	const ts int64 = 1700000000000000
	seedV2DB(t, path, ts, "dom")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	lv := latestVersion()
	if got := userVersion(t, db); got != lv {
		t.Fatalf("user_version = %d, want %d", got, lv)
	}

	// One per-extractor run, grouped from the legacy "dom" output under "wget".
	var runID, snapshotID int64
	var extractor, status string
	if err := db.QueryRowContext(ctx, `
		SELECT id, snapshot_id, extractor, status FROM extractor_runs`).Scan(
		&runID, &snapshotID, &extractor, &status); err != nil {
		t.Fatalf("query extractor_runs: %v", err)
	}
	if extractor != "wget" {
		t.Errorf("extractor = %q, want wget", extractor)
	}
	if status != "succeeded" {
		t.Errorf("status = %q, want succeeded", status)
	}
	var snapID int64
	if err := db.QueryRowContext(ctx, "SELECT id FROM snapshots WHERE timestamp = ?", ts).Scan(&snapID); err != nil {
		t.Fatalf("query snapshot id: %v", err)
	}
	if snapshotID != snapID {
		t.Errorf("snapshot_id = %d, want %d", snapshotID, snapID)
	}

	// One step_outputs row carrying the legacy output name and filename.
	var name, filename string
	if err := db.QueryRowContext(ctx, `
		SELECT name, COALESCE(filename, '') FROM step_outputs WHERE run_id = ?`, runID).Scan(&name, &filename); err != nil {
		t.Fatalf("query step_outputs: %v", err)
	}
	if name != "dom" {
		t.Errorf("step_outputs.name = %q, want dom", name)
	}
	if filename != "output.html" {
		t.Errorf("step_outputs.filename = %q, want output.html", filename)
	}
	assertNoFKViolations(t, db)
}

// TestMigrate_v6OnDeleteCascade verifies that deleting a snapshot removes its
// extractor_runs and step_outputs automatically via ON DELETE CASCADE.
func TestMigrate_v6OnDeleteCascade(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	const ts int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(ctx, "https://example.com", ts)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	runID, err := db.InsertRun(ctx, ExtractorRun{
		SnapshotID: snapshotID,
		Extractor:  "wget",
		Status:     "succeeded",
		StartedAt:  ts,
		FinishedAt: ts + 1000,
	})
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if _, err := db.InsertStepOutput(ctx, runID, StepOutput{
		RunID:    runID,
		Name:     "dom",
		Filename: "output.html",
		Status:   "succeeded",
		StartTs:  ts,
		EndTs:    ts + 500,
	}); err != nil {
		t.Fatalf("InsertStepOutput: %v", err)
	}

	// Deleting the snapshot should cascade to extractor_runs and step_outputs.
	if _, err := db.ExecContext(ctx, "DELETE FROM snapshots WHERE id = ?", snapshotID); err != nil {
		t.Fatalf("DELETE snapshot: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM extractor_runs WHERE snapshot_id = ?", snapshotID).Scan(&count); err != nil {
		t.Fatalf("count extractor_runs: %v", err)
	}
	if count != 0 {
		t.Fatalf("extractor_runs count = %d after snapshot delete, want 0", count)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM step_outputs").Scan(&count); err != nil {
		t.Fatalf("count step_outputs: %v", err)
	}
	if count != 0 {
		t.Fatalf("step_outputs count = %d after snapshot delete, want 0", count)
	}
	assertNoFKViolations(t, db)
}