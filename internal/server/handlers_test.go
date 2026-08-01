package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// seedSnapshots inserts n succeeded snapshots into db for view tests.
func seedSnapshots(t *testing.T, db *meta.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ts := int64(1700000000000000 + i)
		if _, err := db.CreateSnapshot(context.Background(), "https://example.com/"+strconv.Itoa(i), ts); err != nil {
			t.Fatalf("CreateSnapshot: %v", err)
		}
		if err := db.UpdateSnapshot(context.Background(), ts, "Title "+strconv.Itoa(i)); err != nil {
			t.Fatalf("UpdateSnapshot: %v", err)
		}
	}
}

func TestHandleList_empty(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "No snapshots yet.") {
		t.Errorf("body missing empty-state message: %q", rec.Body.String())
	}
}

func TestHandleList_rendersRows(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 3)
	s := &Server{DB: db}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "3 total") {
		t.Errorf("body missing total count: %q", body)
	}
	if !strings.Contains(body, "Title 2") {
		t.Errorf("body missing snapshot title (newest first): %q", body)
	}
	// Detail link uses the ArchiveBox "seconds.microseconds" timestamp path.
	if !strings.Contains(body, "/1700000000.000002") {
		t.Errorf("body missing detail link: %q", body)
	}
}

func TestHandleList_pagination(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 3)
	s := &Server{DB: db}
	r := s.Router()

	// Page size 2, first page: shows prev disabled, next enabled.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?limit=2&offset=0", nil)
	r.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "page 1 of 2") {
		t.Errorf("body missing page indicator: %q", body)
	}
	if !strings.Contains(body, "Next") || !strings.Contains(body, "offset=2") {
		t.Errorf("body missing next link: %q", body)
	}
	// Prev should be present but disabled (gray-300).
	if !strings.Contains(body, "text-gray-300") {
		t.Errorf("body missing disabled prev: %q", body)
	}
}

func TestHandleDetail_found(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	const ts int64 = 1700000000000000

	// Create a snapshot dir with one output file so the file list is populated.
	root := filepath.Join(t.TempDir(), "archive")
	dir := archive.SnapshotDir(root, ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<html>hi</html>"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Seed an extractor run so the per-extractor status table is populated.
	if _, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		Timestamp:  ts,
		Extractor:  "dom",
		Status:     "succeeded",
		Output:     "output.html",
		StartedAt:  ts,
		FinishedAt: ts + 1000,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(ts), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Title 0") {
		t.Errorf("body missing title: %q", body)
	}
	if !strings.Contains(body, "https://example.com/0") {
		t.Errorf("body missing url: %q", body)
	}
	if !strings.Contains(body, "output.html") {
		t.Errorf("body missing file link: %q", body)
	}
	// File link points at the static archive route.
	if !strings.Contains(body, "/archive/"+snapshot.Format(ts)+"/output.html") {
		t.Errorf("body missing archive file path: %q", body)
	}
	// Per-extractor status table is rendered.
	if !strings.Contains(body, "Extractors") {
		t.Errorf("body missing extractors heading: %q", body)
	}
	if !strings.Contains(body, ">dom<") || !strings.Contains(body, "succeeded") {
		t.Errorf("body missing dom/succeeded run row: %q", body)
	}
}

func TestHandleDetail_notFound(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/1700000000.000099", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if !strings.Contains(rec.Body.String(), "Not found") {
		t.Errorf("body missing not-found message: %q", rec.Body.String())
	}
}

func TestHandleDetail_invalidTimestamp(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/not-a-timestamp", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (invalid timestamp -> 404)", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAddForm_renders(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/add", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "Add a URL") {
		t.Errorf("body missing form heading: %q", rec.Body.String())
	}
}

func TestHandleAddSubmit_missingURL(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	rec := httptest.NewRecorder()
	form := "url="
	req := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if !strings.Contains(rec.Body.String(), "URL is required") {
		t.Errorf("body missing error: %q", rec.Body.String())
	}
}

func TestHandleAddSubmit_success(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Example</title></head></html>"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer upstream.Close()

	db := newTestDB(t)
	s := &Server{DB: db, ArchiveRoot: filepath.Join(t.TempDir(), "archive")}
	r := s.Router()

	rec := httptest.NewRecorder()
	form := "url=" + upstream.URL
	req := httptest.NewRequest(http.MethodPost, "/add", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if loc == "" || !strings.HasPrefix(loc, "/") {
		t.Fatalf("Location = %q, want a snapshot path", loc)
	}
	// The redirect target should be a valid ArchiveBox timestamp path.
	if !strings.Contains(loc, ".") {
		t.Errorf("Location %q does not look like a timestamp path", loc)
	}

	// The snapshot was persisted to the DB.
	var n int
	if err := db.QueryRow("SELECT count(*) FROM snapshots WHERE url = ?", upstream.URL).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Errorf("snapshot count = %d, want 1", n)
	}
}
