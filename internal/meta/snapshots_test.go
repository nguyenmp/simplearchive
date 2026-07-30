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
