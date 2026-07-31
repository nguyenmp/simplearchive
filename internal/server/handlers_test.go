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
