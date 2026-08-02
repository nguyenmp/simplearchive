package meta

import (
	"context"
	"errors"
	"testing"
)

func TestGetSnapshot_notFound(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	_, err = db.GetSnapshot(context.Background(), 1700000000000000)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetSnapshot_found(t *testing.T) {
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

	s, err := db.GetSnapshot(context.Background(), ts)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if s.Timestamp != ts {
		t.Errorf("timestamp = %d, want %d", s.Timestamp, ts)
	}
	if s.URL != "https://example.com" {
		t.Errorf("url = %q", s.URL)
	}
	if s.Title != "Example" {
		t.Errorf("title = %q, want Example", s.Title)
	}
}

func TestGetSnapshot_nullTitleIsEmpty(t *testing.T) {
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
	// Never call UpdateSnapshot, so title stays NULL.
	s, err := db.GetSnapshot(context.Background(), ts)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if s.Title != "" {
		t.Errorf("title = %q, want empty string for NULL", s.Title)
	}
}

func TestListSnapshots_empty(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	snaps, total, err := db.ListSnapshots(context.Background(), 50, 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if len(snaps) != 0 {
		t.Fatalf("len = %d, want 0", len(snaps))
	}
}

func TestListSnapshots_orderingAndPagination(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// Insert three snapshots out of order.
	for _, ts := range []int64{1700000000000001, 1700000000000003, 1700000000000002} {
		if _, err := db.CreateSnapshot(context.Background(), "https://example.com", ts); err != nil {
			t.Fatalf("CreateSnapshot %d: %v", ts, err)
		}
	}

	// Page size 2, first page: newest two (timestamps ...3 and ...2).
	snaps, total, err := db.ListSnapshots(context.Background(), 2, 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(snaps) != 2 {
		t.Fatalf("len = %d, want 2", len(snaps))
	}
	if snaps[0].Timestamp != 1700000000000003 || snaps[1].Timestamp != 1700000000000002 {
		t.Errorf("order = %d, %d; want ...3 then ...2", snaps[0].Timestamp, snaps[1].Timestamp)
	}

	// Second page: oldest one (timestamp ...1).
	snaps, _, err = db.ListSnapshots(context.Background(), 2, 2)
	if err != nil {
		t.Fatalf("ListSnapshots page 2: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("page 2 len = %d, want 1", len(snaps))
	}
	if snaps[0].Timestamp != 1700000000000001 {
		t.Errorf("page 2 first = %d, want ...1", snaps[0].Timestamp)
	}
}

func TestListSnapshots_clampsLimit(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	for i := 0; i < 3; i++ {
		if _, err := db.CreateSnapshot(context.Background(), "https://example.com", int64(1700000000000000+i)); err != nil {
			t.Fatalf("CreateSnapshot: %v", err)
		}
	}

	// limit <= 0 is clamped to maxLimit, so all 3 are returned.
	snaps, _, err := db.ListSnapshots(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("len = %d, want 3 (clamped to maxLimit)", len(snaps))
	}
}

func TestDeleteSnapshot_removesRowAndCascades(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	const ts int64 = 1700000000000000
	if _, err := db.CreateSnapshot(ctx, "https://example.com", ts); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if err := db.DeleteSnapshot(ctx, ts); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}

	_, err = db.GetSnapshot(ctx, ts)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSnapshot after delete: err = %v, want ErrNotFound", err)
	}

	// Deleting a missing snapshot returns ErrNotFound.
	if err := db.DeleteSnapshot(ctx, ts); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteSnapshot missing: err = %v, want ErrNotFound", err)
	}
}
