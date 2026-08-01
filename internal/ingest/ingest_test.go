package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors/headers"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

func TestAdd_archivesSnapshot(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Example</title></head><body>hi</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	root := filepath.Join(t.TempDir(), "archive")
	res, err := Add(context.Background(), db, root, srv.URL)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Title != "Example" {
		t.Errorf("title = %q, want Example", res.Title)
	}

	// The snapshot's title is recorded (a snapshot has no stored status; its
	// succeeded state is derived from its extractor_runs).
	var title string
	if err := db.QueryRow("SELECT title FROM snapshots WHERE timestamp = ?", res.Timestamp).Scan(&title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if title != "Example" {
		t.Errorf("title = %q, want Example", title)
	}

	// On-disk outputs exist.
	dir := archive.SnapshotDir(root, res.Timestamp)
	if got, err := os.ReadFile(filepath.Join(dir, wget.OutputFile)); err != nil || string(got) != body {
		t.Errorf("output.html = %q, err=%v", got, err)
	}
	if _, err := os.ReadFile(filepath.Join(dir, headers.OutputFile)); err != nil {
		t.Errorf("headers.json missing: %v", err)
	}
	if _, err := os.ReadFile(filepath.Join(dir, archive.IndexFile)); err != nil {
		t.Errorf("index.json missing: %v", err)
	}

	// Dir field is the snapshot directory.
	if !strings.HasSuffix(res.Dir, snapshot.Format(res.Timestamp)) {
		t.Errorf("Dir = %q, want suffix %q", res.Dir, snapshot.Format(res.Timestamp))
	}

	// The always-on extractors each record a succeeded run. (chromedp adds
	// three more only when built with the "chromedp" tag, so assert by name
	// rather than by exact count.)
	runs, err := db.ListRunsBySnapshot(context.Background(), res.SnapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	byExtractor := make(map[string]meta.ExtractorRun, len(runs))
	for _, r := range runs {
		byExtractor[r.Extractor] = r
	}
	for _, name := range []string{"wget", "headers", "obelisk"} {
		r, ok := byExtractor[name]
		if !ok {
			t.Errorf("missing run for %q", name)
			continue
		}
		if r.Status != "succeeded" {
			t.Errorf("run %q status = %q, want succeeded", name, r.Status)
		}
	}
	// The wget run produced a "dom" output (the primary DOM fetch).
	if r, ok := byExtractor["wget"]; ok {
		foundDom := false
		for _, o := range r.Outputs {
			if o.Name == "dom" {
				foundDom = true
			}
		}
		if !foundDom {
			t.Errorf("wget run missing a dom output: %+v", r.Outputs)
		}
	}
}

func TestAdd_domFailure_recordsFailedRun(t *testing.T) {
	t.Parallel()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	root := filepath.Join(t.TempDir(), "archive")
	// Steps are independent (no primary-fatal): Add does not error when the DOM
	// fetch fails — it runs every extractor and records per-step status.
	res, err := Add(context.Background(), db, root, "http://127.0.0.1:1/no-such-port")
	if err != nil {
		t.Fatalf("Add returned error %v; steps are independent and should not fail Add", err)
	}

	runs, err := db.ListRunsBySnapshot(context.Background(), res.SnapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	byExtractor := make(map[string]meta.ExtractorRun, len(runs))
	for _, r := range runs {
		byExtractor[r.Extractor] = r
	}
	r, ok := byExtractor["wget"]
	if !ok {
		t.Fatalf("missing wget run: %+v", runs)
	}
	if r.Status != "failed" {
		t.Errorf("wget run status = %q, want failed", r.Status)
	}
	if r.Error == "" {
		t.Errorf("wget run error = empty, want a failure cause")
	}

	// The DOM fetch failed, so there is no title.
	if res.Title != "" {
		t.Errorf("title = %q, want empty (no DOM)", res.Title)
	}
}

func TestEnqueue_thenRunSnapshot_drainsPendingRuns(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Example</title></head><body>hi</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()
	root := filepath.Join(t.TempDir(), "archive")

	snapshotID, ts, err := Enqueue(context.Background(), db, srv.URL)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Enqueue creates one pending run per default-pipeline extractor.
	runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != len(defaultPipeline()) {
		t.Fatalf("pending runs = %d, want %d", len(runs), len(defaultPipeline()))
	}
	for _, r := range runs {
		if r.Status != "pending" {
			t.Errorf("run %q status = %q, want pending", r.Extractor, r.Status)
		}
	}

	res, err := RunSnapshot(context.Background(), db, root, snapshotID)
	if err != nil {
		t.Fatalf("RunSnapshot: %v", err)
	}
	if res.Timestamp != ts {
		t.Errorf("timestamp = %d, want %d", res.Timestamp, ts)
	}
	if res.Title != "Example" {
		t.Errorf("title = %q, want Example", res.Title)
	}

	// All runs are now terminal; the wget run succeeded with a dom output.
	runs, err = db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	byExtractor := make(map[string]meta.ExtractorRun, len(runs))
	for _, r := range runs {
		byExtractor[r.Extractor] = r
		if r.Status == "pending" || r.Status == "running" {
			t.Errorf("run %q still %q", r.Extractor, r.Status)
		}
	}
	if r, ok := byExtractor["wget"]; !ok || r.Status != "succeeded" {
		t.Errorf("wget run = %+v, want succeeded", r)
	} else {
		foundDom := false
		for _, o := range r.Outputs {
			if o.Name == "dom" {
				foundDom = true
			}
		}
		if !foundDom {
			t.Errorf("wget run missing dom output: %+v", r.Outputs)
		}
	}

	// index.json was rebuilt and records the dom output.
	idx, err := os.ReadFile(filepath.Join(res.Dir, archive.IndexFile))
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	if !strings.Contains(string(idx), `"dom"`) {
		t.Errorf("index.json missing dom entry: %s", idx)
	}
}

func TestRunNext_drainsQueueOneAtATime(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Example</title></head><body>hi</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()
	root := filepath.Join(t.TempDir(), "archive")

	// Enqueue two snapshots; neither is archived yet (runs are pending).
	id1, _, err := Enqueue(context.Background(), db, srv.URL)
	if err != nil {
		t.Fatalf("Enqueue 1: %v", err)
	}
	id2, _, err := Enqueue(context.Background(), db, srv.URL)
	if err != nil {
		t.Fatalf("Enqueue 2: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("Enqueue returned the same snapshot id twice: %d", id1)
	}

	// RunNext claims and archives one snapshot per call, oldest first.
	for i := 0; i < 2; i++ {
		ran, err := RunNext(context.Background(), db, root)
		if err != nil {
			t.Fatalf("RunNext %d: %v", i, err)
		}
		if !ran {
			t.Fatalf("RunNext %d = ran=false, want true (queue not empty)", i)
		}
	}
	// Queue is now empty.
	ran, err := RunNext(context.Background(), db, root)
	if err != nil {
		t.Fatalf("RunNext drain: %v", err)
	}
	if ran {
		t.Errorf("RunNext = ran=true, want false on empty queue")
	}

	// Both snapshots are fully terminal.
	for _, id := range []int64{id1, id2} {
		runs, err := db.ListRunsBySnapshot(context.Background(), id)
		if err != nil {
			t.Fatalf("ListRunsBySnapshot %d: %v", id, err)
		}
		for _, r := range runs {
			if r.Status == "pending" || r.Status == "running" {
				t.Errorf("snapshot %d run %q still %q", id, r.Extractor, r.Status)
			}
		}
	}
}
