package meta

import (
	"context"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
)

func TestCreateSnapshot_insertsRow(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	id, err := db.CreateSnapshot(context.Background(), "https://example.com", 1700000000000000)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if id == 0 {
		t.Fatal("CreateSnapshot returned id 0")
	}

	var url string
	err = db.db.QueryRowContext(context.Background(),
		"SELECT url FROM snapshots WHERE timestamp = ?", 1700000000000000,
	).Scan(&url)
	if err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("url = %q, want https://example.com", url)
	}
	// A snapshot has no stored status; it derives from its extractor_runs.
	if _, err := db.db.Exec("SELECT status FROM snapshots"); err == nil {
		t.Errorf("snapshots.status column should not exist")
	}
}

func TestUpdateSnapshot_setsTitle(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000000000
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", ts); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := db.UpdateSnapshot(context.Background(), ts, "Example"); err != nil {
		t.Fatalf("UpdateSnapshot: %v", err)
	}

	var title any
	if err := db.db.QueryRowContext(context.Background(),
		"SELECT title FROM snapshots WHERE timestamp = ?", ts,
	).Scan(&title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != "Example" {
		t.Errorf("title = %v, want Example", title)
	}
}

func TestUpdateSnapshot_nullTitleWhenEmpty(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000000001
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", ts); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := db.UpdateSnapshot(context.Background(), ts, ""); err != nil {
		t.Fatalf("UpdateSnapshot: %v", err)
	}

	var title any
	if err := db.db.QueryRowContext(context.Background(),
		"SELECT title FROM snapshots WHERE timestamp = ?", ts).Scan(&title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != nil {
		t.Errorf("title = %v, want nil", title)
	}
}

func TestUpdateSnapshot_missingRowErrors(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.UpdateSnapshot(context.Background(), 9999999999999999, "x"); err == nil {
		t.Fatal("UpdateSnapshot on missing row returned nil error")
	}
}

func TestUpsertSnapshot_insertsNewRow(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	e := archive.IndexEntry{
		Timestamp:  1728277530511000,
		URL:        "https://example.com",
		Title:      "Example",
		IsArchived: true,
	}
	if err := db.UpsertSnapshot(context.Background(), e); err != nil {
		t.Fatalf("UpsertSnapshot: %v", err)
	}

	var url, title string
	var createdAt, updatedAt int64
	if err := db.db.QueryRowContext(context.Background(),
		"SELECT url, title, created_at, updated_at FROM snapshots WHERE timestamp = ?",
		e.Timestamp,
	).Scan(&url, &title, &createdAt, &updatedAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("url = %q", url)
	}
	if title != "Example" {
		t.Errorf("title = %q, want Example", title)
	}
	if createdAt != e.Timestamp {
		t.Errorf("created_at = %d, want %d", createdAt, e.Timestamp)
	}
	if updatedAt != e.Timestamp {
		t.Errorf("updated_at = %d, want %d", updatedAt, e.Timestamp)
	}
}

func TestUpsertSnapshot_nullTitleWhenEmpty(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := db.UpsertSnapshot(context.Background(), archive.IndexEntry{
		Timestamp: 1728277530511000,
		URL:       "https://example.com",
	}); err != nil {
		t.Fatalf("UpsertSnapshot: %v", err)
	}
	var title any
	if err := db.db.QueryRowContext(context.Background(),
		"SELECT title FROM snapshots WHERE timestamp = ?", 1728277530511000).Scan(&title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != nil {
		t.Errorf("title = %v, want nil", title)
	}
}

func TestUpsertSnapshot_isIdempotentAndPreservesCreatedAt(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1728277530511000
	first := archive.IndexEntry{Timestamp: ts, URL: "https://old.example.com", Title: "Old", IsArchived: true}
	if err := db.UpsertSnapshot(context.Background(), first); err != nil {
		t.Fatalf("UpsertSnapshot (1): %v", err)
	}
	var createdAtAfterFirst int64
	_ = db.db.QueryRowContext(context.Background(),
		"SELECT created_at FROM snapshots WHERE timestamp = ?", ts).Scan(&createdAtAfterFirst)

	// Re-import with refreshed fields; created_at must not change.
	second := archive.IndexEntry{Timestamp: ts, URL: "https://new.example.com", Title: "New", IsArchived: true}
	if err := db.UpsertSnapshot(context.Background(), second); err != nil {
		t.Fatalf("UpsertSnapshot (2): %v", err)
	}

	var count, url, title string
	if err := db.db.QueryRowContext(context.Background(),
		"SELECT printf('%d', count(*)), url, title FROM snapshots WHERE timestamp = ?", ts,
	).Scan(&count, &url, &title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != "1" {
		t.Fatalf("row count = %s, want 1", count)
	}
	if url != "https://new.example.com" {
		t.Errorf("url = %q, want refreshed", url)
	}
	if title != "New" {
		t.Errorf("title = %q, want New", title)
	}

	var createdAtAfterSecond int64
	_ = db.db.QueryRowContext(context.Background(),
		"SELECT created_at FROM snapshots WHERE timestamp = ?", ts).Scan(&createdAtAfterSecond)
	if createdAtAfterSecond != createdAtAfterFirst {
		t.Errorf("created_at changed: %d -> %d", createdAtAfterFirst, createdAtAfterSecond)
	}
}
