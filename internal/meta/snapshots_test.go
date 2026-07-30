package meta

import (
	"context"
	"testing"
)

func TestCreateSnapshot_insertsPendingRow(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ts, err := db.CreateSnapshot(context.Background(), "https://example.com", 1700000000000)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if ts != 1700000000000 {
		t.Fatalf("ts = %d, want 1700000000000", ts)
	}

	var url, status string
	var isArchived int
	err = db.QueryRowContext(context.Background(),
		"SELECT url, status, is_archived FROM snapshots WHERE timestamp = ?", ts,
	).Scan(&url, &status, &isArchived)
	if err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if url != "https://example.com" {
		t.Errorf("url = %q, want https://example.com", url)
	}
	if status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
	if isArchived != 0 {
		t.Errorf("is_archived = %d, want 0", isArchived)
	}
}

func TestUpdateSnapshot_marksSucceeded(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000000
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", ts); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := db.UpdateSnapshot(context.Background(), ts, "Example"); err != nil {
		t.Fatalf("UpdateSnapshot: %v", err)
	}

	var status, title any
	var isArchived int
	if err := db.QueryRowContext(context.Background(),
		"SELECT status, title, is_archived FROM snapshots WHERE timestamp = ?", ts,
	).Scan(&status, &title, &isArchived); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "succeeded" {
		t.Errorf("status = %v, want succeeded", status)
	}
	if title != "Example" {
		t.Errorf("title = %v, want Example", title)
	}
	if isArchived != 1 {
		t.Errorf("is_archived = %d, want 1", isArchived)
	}
}

func TestUpdateSnapshot_nullTitleWhenEmpty(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000001
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", ts); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := db.UpdateSnapshot(context.Background(), ts, ""); err != nil {
		t.Fatalf("UpdateSnapshot: %v", err)
	}

	var title any
	if err := db.QueryRowContext(context.Background(),
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

	if err := db.UpdateSnapshot(context.Background(), 9999999999999, "x"); err == nil {
		t.Fatal("UpdateSnapshot on missing row returned nil error")
	}
}
