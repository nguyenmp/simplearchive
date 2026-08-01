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

	// DB row is marked succeeded.
	var status, title string
	if err := db.QueryRow("SELECT status, title FROM snapshots WHERE timestamp = ?", res.Timestamp).Scan(&status, &title); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "succeeded" {
		t.Errorf("status = %q, want succeeded", status)
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
	runs, err := db.ListRunsByTimestamp(context.Background(), res.Timestamp)
	if err != nil {
		t.Fatalf("ListRunsByTimestamp: %v", err)
	}
	byName := make(map[string]meta.ExtractorRun, len(runs))
	for _, r := range runs {
		byName[r.Extractor] = r
	}
	for _, name := range []string{"dom", "headers", "singlefile"} {
		r, ok := byName[name]
		if !ok {
			t.Errorf("missing run for %q", name)
			continue
		}
		if r.Status != "succeeded" {
			t.Errorf("run %q status = %q, want succeeded", name, r.Status)
		}
	}
}

func TestAdd_domFailure_marksFailed(t *testing.T) {
	t.Parallel()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	defer db.Close()

	root := filepath.Join(t.TempDir(), "archive")
	_, err = Add(context.Background(), db, root, "http://127.0.0.1:1/no-such-port")
	if err == nil {
		t.Fatal("Add on unreachable URL returned nil error")
	}

	// Exactly one snapshot row, marked failed (not stuck in pending).
	var status string
	var ts int64
	if err := db.QueryRow("SELECT status, timestamp FROM snapshots").Scan(&status, &ts); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q, want failed", status)
	}

	// Only the primary DOM run was recorded (best-effort extractors did not run).
	runs, err := db.ListRunsByTimestamp(context.Background(), ts)
	if err != nil {
		t.Fatalf("ListRunsByTimestamp: %v", err)
	}
	if len(runs) != 1 || runs[0].Extractor != "dom" || runs[0].Status != "failed" {
		t.Fatalf("runs = %+v, want one failed dom run", runs)
	}
}
