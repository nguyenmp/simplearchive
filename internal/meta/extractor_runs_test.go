package meta

import (
	"context"
	"testing"
)

func TestInsertRun_andListByTimestamp(t *testing.T) {
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

	if _, err := db.InsertRun(context.Background(), ExtractorRun{
		Timestamp:  ts,
		Extractor: "dom",
		Status:    "succeeded",
		Output:    "output.html",
		StartedAt: ts,
		FinishedAt: ts + 1000,
	}); err != nil {
		t.Fatalf("InsertRun dom: %v", err)
	}
	id2, err := db.InsertRun(context.Background(), ExtractorRun{
		Timestamp:  ts,
		Extractor:  "favicon",
		Status:     "failed",
		Output:     "",
		StartedAt:  ts,
		FinishedAt: 0, // NULL
		Error:      "wget: exit status 8",
	})
	if err != nil {
		t.Fatalf("InsertRun favicon: %v", err)
	}
	if id2 == 0 {
		t.Fatal("InsertRun returned id 0")
	}

	runs, err := db.ListRunsByTimestamp(context.Background(), ts)
	if err != nil {
		t.Fatalf("ListRunsByTimestamp: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}
	if runs[0].Extractor != "dom" || runs[0].Status != "succeeded" || runs[0].Output != "output.html" {
		t.Errorf("runs[0] = %+v", runs[0])
	}
	if runs[0].FinishedAt != ts+1000 {
		t.Errorf("runs[0].FinishedAt = %d, want %d", runs[0].FinishedAt, ts+1000)
	}
	if runs[1].Extractor != "favicon" || runs[1].Status != "failed" {
		t.Errorf("runs[1] = %+v", runs[1])
	}
	if runs[1].FinishedAt != 0 {
		t.Errorf("runs[1].FinishedAt = %d, want 0 (NULL)", runs[1].FinishedAt)
	}
	if runs[1].Error != "wget: exit status 8" {
		t.Errorf("runs[1].Error = %q", runs[1].Error)
	}
}

func TestListRunsByTimestamp_empty(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	runs, err := db.ListRunsByTimestamp(context.Background(), 1700000000000000)
	if err != nil {
		t.Fatalf("ListRunsByTimestamp: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len = %d, want 0", len(runs))
	}
}
