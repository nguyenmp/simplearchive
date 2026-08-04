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

	const timestamp int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp)
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
		StartedAt:  timestamp,
		FinishedAt: timestamp + 1000,
	})
	if err != nil {
		t.Fatalf("InsertRun wget: %v", err)
	}
	if runID == 0 {
		t.Fatal("InsertRun returned id 0")
	}
	for _, out := range []StepOutput{
		{RunID: runID, Name: "dom", Filename: "output.html", Status: "succeeded", StartTs: timestamp, EndTs: timestamp + 500},
		{RunID: runID, Name: "favicon", Filename: "favicon.ico", Status: "succeeded", StartTs: timestamp, EndTs: timestamp + 1000},
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
		StartedAt:  timestamp,
		Error:      "wget: exit status 8",
	})
	if err != nil {
		t.Fatalf("InsertRun favicon: %v", err)
	}
	if _, err := db.InsertStepOutput(context.Background(), runID2, StepOutput{
		RunID: runID2, Name: "favicon", Status: "failed", StartTs: timestamp, Error: "wget: exit status 8",
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
	if runs[0].FinishedAt != timestamp+1000 {
		t.Errorf("runs[0].FinishedAt = %d, want %d", runs[0].FinishedAt, timestamp+1000)
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

	const timestamp int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	runID, err := db.InsertRun(context.Background(), ExtractorRun{
		SnapshotID: snapshotID, Extractor: "wget", Status: "succeeded", StartedAt: timestamp, FinishedAt: timestamp,
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

	const timestamp int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp)
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

func TestDeleteStepOutputsByFilename(t *testing.T) {
	t.Parallel()
	db, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	const timestamp int64 = 1700000000000000
	snapshotID, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp)
	if err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	runID, err := db.InsertRun(context.Background(), ExtractorRun{
		SnapshotID: snapshotID, Extractor: "wget", Status: "succeeded", StartedAt: timestamp, FinishedAt: timestamp,
	})
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	// Two outputs: output.html is deleted by filename; favicon.ico survives.
	for _, out := range []StepOutput{
		{RunID: runID, Name: "dom", Filename: "output.html", Status: "succeeded"},
		{RunID: runID, Name: "favicon", Filename: "favicon.ico", Status: "succeeded"},
	} {
		if _, err := db.InsertStepOutput(context.Background(), runID, out); err != nil {
			t.Fatalf("InsertStepOutput %s: %v", out.Name, err)
		}
	}
	// A second snapshot's outputs must be untouched by the delete.
	otherID, err := db.CreateSnapshot(context.Background(), "https://other.example", timestamp+1)
	if err != nil {
		t.Fatalf("CreateSnapshot other: %v", err)
	}
	otherRunID, err := db.InsertRun(context.Background(), ExtractorRun{
		SnapshotID: otherID, Extractor: "wget", Status: "succeeded", StartedAt: timestamp, FinishedAt: timestamp,
	})
	if err != nil {
		t.Fatalf("InsertRun other: %v", err)
	}
	if _, err := db.InsertStepOutput(context.Background(), otherRunID, StepOutput{
		RunID: otherRunID, Name: "dom", Filename: "output.html", Status: "succeeded",
	}); err != nil {
		t.Fatalf("InsertStepOutput other: %v", err)
	}

	if err := db.DeleteStepOutputsByFilename(context.Background(), snapshotID, "output.html"); err != nil {
		t.Fatalf("DeleteStepOutputsByFilename: %v", err)
	}

	runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != 1 || len(runs[0].Outputs) != 1 {
		t.Fatalf("outputs after delete = %+v, want 1 (favicon)", runs)
	}
	if got := runs[0].Outputs[0]; got.Name != "favicon" || got.Filename != "favicon.ico" {
		t.Errorf("surviving output = %+v, want favicon.ico", got)
	}

	// The other snapshot's output.html row is untouched.
	otherRuns, err := db.ListRunsBySnapshot(context.Background(), otherID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot other: %v", err)
	}
	if len(otherRuns) != 1 || len(otherRuns[0].Outputs) != 1 || otherRuns[0].Outputs[0].Filename != "output.html" {
		t.Errorf("other snapshot outputs = %+v, want output.html intact", otherRuns)
	}

	// Deleting a filename with no matching rows is a no-op, not an error.
	if err := db.DeleteStepOutputsByFilename(context.Background(), snapshotID, "no-such-file"); err != nil {
		t.Errorf("DeleteStepOutputsByFilename with no match: %v", err)
	}
}
