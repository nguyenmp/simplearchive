package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/ingest"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// newTestDB returns an in-memory SQLite database suitable for tests.
func newTestDB(t *testing.T) *meta.DB {
	t.Helper()
	db, err := meta.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("meta.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHandleHealthz(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q, want application/json", got)
	}
	if body := rec.Body.String(); body != `{"ok":true}` {
		t.Errorf("body = %q, want {\"ok\":true}", body)
	}
}

func TestHandleHealthz_HEAD(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("HEAD content-type = %q, want application/json", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("HEAD body should be empty, got %q", rec.Body.String())
	}
	cl := rec.Header().Get("Content-Length")
	if cl == "" {
		t.Errorf("HEAD Content-Length is empty")
	} else {
		n, err := strconv.Atoi(cl)
		if err != nil || n <= 0 {
			t.Errorf("HEAD Content-Length = %q, want positive integer", cl)
		}
	}
}

func TestServeStatic_tailwindCSS(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/tailwind.css", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.Len(); got == 0 {
		t.Fatal("tailwind.css body is empty")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("content-type = %q, want text/css", ct)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff = %q, want nosniff", got)
	}
}

func TestServeStatic_missingFile(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/does-not-exist.css", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRun_noDB_returnsError(t *testing.T) {
	t.Parallel()
	s := &Server{}
	if err := s.Run(context.Background()); err == nil {
		t.Fatal("Run with nil DB returned nil error")
	}
}

// TestRun_servesOverListener starts a real server on an ephemeral port and
// verifies it answers /healthz over TCP, then shuts down cleanly.
func TestRun_servesOverListener(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &Server{
		DB:       newTestDB(t),
		Listener: ln,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	// Give the goroutine a moment to start serving.
	addr := ln.Addr().String()
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// TestRunWorker_drainsEnqueuedSnapshot enqueues a URL and verifies the serve
// worker goroutine archives it asynchronously (the web Add-URL form's path).
func TestRunWorker_drainsEnqueuedSnapshot(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Example</title></head><body>hi</body></html>"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()

	db := newTestDB(t)
	root := filepath.Join(t.TempDir(), "archive")
	s := &Server{DB: db, ArchiveRoot: root}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.runWorker(ctx)

	snapshotID, _, err := ingest.Enqueue(context.Background(), db, upstream.URL)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Poll until the worker finishes the snapshot (runs all terminal).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
		if err != nil {
			t.Fatalf("ListRunsBySnapshot: %v", err)
		}
		allTerminal := len(runs) > 0
		for _, r := range runs {
			if r.Status == extractors.StatusPending || r.Status == extractors.StatusRunning {
				allTerminal = false
			}
		}
		if allTerminal {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	runs, err := db.ListRunsBySnapshot(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) == 0 {
		t.Fatalf("no runs archived; worker did not drain the snapshot")
	}
	for _, r := range runs {
		if r.Status == extractors.StatusPending || r.Status == extractors.StatusRunning {
			t.Errorf("run %q still %q after worker drain", r.Extractor, r.Status)
		}
	}

	// The DOM fetch succeeded and wrote index.json to disk.
	snap, err := db.GetSnapshotByID(context.Background(), snapshotID)
	if err != nil {
		t.Fatalf("GetSnapshotByID: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, snapshot.Format(snap.Timestamp), "index.json")); err != nil {
		t.Errorf("index.json missing on disk: %v", err)
	}
}

func TestHandleDelete_removesSnapshot(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	root := filepath.Join(t.TempDir(), "archive")
	// Create a fake on-disk directory so we also exercise archive removal.
	dir := filepath.Join(root, snapshot.Format(timestamp))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte("{}"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	// POST delete.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/"+snapshot.Format(timestamp)+"/delete", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d (SeeOther)", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("location = %q, want /", loc)
	}

	// Snapshot is gone.
	_, err := db.GetSnapshot(context.Background(), timestamp)
	if err == nil {
		t.Fatal("GetSnapshot after delete: expected error, got nil")
	}

	// On-disk directory is gone.
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("archive dir %q still exists after delete", dir)
	}
}

// TestHandleList_showsFileCountAndSize verifies that the list page renders
// file counts and size for each snapshot.
func TestHandleList_showsFileCountAndSize(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	root := filepath.Join(t.TempDir(), "archive")
	dir := filepath.Join(root, snapshot.Format(timestamp))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<html></html>"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "favicon.ico"), []byte("ico"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "2 files") {
		t.Errorf("list body missing file count; want '2 files' in body, got: %s", body)
	}
	if !strings.Contains(body, "B") {
		t.Errorf("list body missing size; want human size in body")
	}
}

func TestHandleDelete_notFound(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/1700000000000000/delete", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleDetail_showsNestedFiles verifies that files inside subdirectories
// (e.g. media/video.mp4) appear in the detail page as OtherFiles.
func TestHandleDetail_showsNestedFiles(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	root := filepath.Join(t.TempDir(), "archive")
	dir := filepath.Join(root, snapshot.Format(timestamp))
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<html></html>"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "video.mp4"), []byte("fakevideo"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile nested: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "media/video.mp4") {
		t.Errorf("detail body missing nested file; want media/video.mp4 in body")
	}
	if !strings.Contains(body, "output.html") {
		t.Errorf("detail body missing top-level file; want output.html in body")
	}
	// File count and size
	if !strings.Contains(body, "2 files") {
		t.Errorf("detail body missing file count; want '2 files' in body")
	}
	if !strings.Contains(body, "B") {
		t.Errorf("detail body missing size; want human size in body")
	}
}

// TestHandleDetail_showsIndividualFileSizes verifies that each OtherFiles
// entry in the detail page shows its individual file size.
func TestHandleDetail_showsIndividualFileSizes(t *testing.T) {
	t.Parallel()
	const testContent = "hello world"
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	root := filepath.Join(t.TempDir(), "archive")
	dir := filepath.Join(root, snapshot.Format(timestamp))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte(testContent), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	wantSize := humanSize(int64(len(testContent)))
	if !strings.Contains(body, wantSize) {
		t.Errorf("detail body missing individual file size; want %q in body, got:\n%s", wantSize, body)
	}
}
