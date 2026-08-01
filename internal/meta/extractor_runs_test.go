package meta

import (
	"context"
	"testing"
)

func TestInsertRun_andListBySnapshot(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", ts)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	if snapshotID == 0 {
		t.Fatal("CreateSnapshot returned id 0")
	}

	// wget run produces two outputs (dom + favicon); the run aggregates them.
	runID, err := db.InsertRun(context.Background(), ExtractorRun{
		SnapshotID: snapshotID,
		Extractor:  "wget",
		Status:     "succeeded",
		StartedAt:  ts,
		FinishedAt: ts + 1000,
	})
	if err != nil {
		t.Fatalf("InsertRun wget: %v", err)
	}
	if runID == 0 {
		t.Fatal("InsertRun returned id 0")
	}
	for _, out := range []StepOutput{
		{RunID: runID, Name: "dom", Filename: "output.html", Status: "succeeded", StartTs: ts, EndTs: ts + 500},
		{RunID: runID, Name: "favicon", Filename: "favicon.ico", Status: "succeeded", StartTs: ts, EndTs: ts + 1000},
	} {
		if _, err := db.InsertStepOutput(context.Background(), runID, out); err != nil {
			t.Fatalf("InsertStepOutput %s: %v", out.Name, err)
		}
	}

	// A failed favicon-only run records a NULL finished_at and an error.
	runID2, err := db.InsertRun(context.Background(), ExtractorRun{
		SnapshotID: snapshotID,
		Extractor:  "wget-favicon",
		Status:     "failed",
		StartedAt:  ts,
		Error:      "wget: exit status 8",
	})
	if err != nil {
		t.Fatalf("InsertRun favicon: %v", err)
	}
	if _, err := db.InsertStepOutput(context.Background(), runID2, StepOutput{
		RunID: runID2, Name: "favicon", Status: "failed", StartTs: ts, Error: "wget: exit status 8",
	}); err != nil {
		t.Fatalf("InsertStepOutput favicon: %v", err)
	}

	runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len = %d, want 2", len(runs))
	}

	if runs[0].Extractor != "wget" || runs[0].Status != "succeeded" {
		t.Errorf("runs[0] = %+v", runs[0])
	}
	if runs[0].FinishedAt != ts+1000 {
		t.Errorf("runs[0].FinishedAt = %d, want %d", runs[0].FinishedAt, ts+1000)
	}
	if len(runs[0].Outputs) != 2 {
		t.Fatalf("runs[0] outputs = %d, want 2", len(runs[0].Outputs))
	}
	if runs[0].Outputs[0].Name != "dom" || runs[0].Outputs[0].Filename != "output.html" {
		t.Errorf("runs[0].Outputs[0] = %+v", runs[0].Outputs[0])
	}
	if runs[0].Outputs[1].Name != "favicon" || runs[0].Outputs[1].Filename != "favicon.ico" {
		t.Errorf("runs[0].Outputs[1] = %+v", runs[0].Outputs[1])
	}

	if runs[1].Extractor != "wget-favicon" || runs[1].Status != "failed" {
		t.Errorf("runs[1] = %+v", runs[1])
	}
	if runs[1].FinishedAt != 0 {
		t.Errorf("runs[1].FinishedAt = %d, want 0 (NULL)", runs[1].FinishedAt)
	}
	if runs[1].Error != "wget: exit status 8" {
		t.Errorf("runs[1].Error = %q", runs[1].Error)
	}
	if len(runs[1].Outputs) != 1 || runs[1].Outputs[0].Name != "favicon" {
		t.Errorf("runs[1].Outputs = %+v", runs[1].Outputs)
	}
}

func TestInsertStepOutput_marshalCmd(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", ts)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	runID, err := db.InsertRun(context.Background(), ExtractorRun{
		SnapshotID: snapshotID, Extractor: "wget", Status: "succeeded", StartedAt: ts, FinishedAt: ts,
	})
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if _, err := db.InsertStepOutput(context.Background(), runID, StepOutput{
		RunID: runID, Name: "dom", Filename: "output.html", Status: "succeeded",
		Cmd: []string{"wget", "--no-verbose", "https://example.com"},
	}); err != nil {
		t.Fatalf("InsertStepOutput: %v", err)
	}

	runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != 1 || len(runs[0].Outputs) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
	cmd := runs[0].Outputs[0].Cmd
	if len(cmd) != 3 || cmd[0] != "wget" || cmd[2] != "https://example.com" {
		t.Errorf("cmd = %#v, want [wget --no-verbose https://example.com]", cmd)
	}
}

func TestListRunsBySnapshot_empty(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const ts int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", ts)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len = %d, want 0", len(runs))
	}
}
