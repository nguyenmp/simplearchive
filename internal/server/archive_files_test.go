package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// newArchiveServer seeds a snapshot dir with one file and returns a server
// rooted at it, plus the snapshot's formatted timestamp path.
func newArchiveServer(t *testing.T) (*Server, string) {
	t.Helper()
	db := newTestDB(t)
	const ts int64 = 1700000000000000
	root := filepath.Join(t.TempDir(), "archive")
	dir := archive.SnapshotDir(root, ts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<html>hi</html>"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>index</html>"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write index: %v", err)
	}
	return &Server{DB: db, ArchiveRoot: root}, snapshot.Format(ts)
}

func TestHandleArchiveFile_servesFile(t *testing.T) {
	t.Parallel()
	s, ts := newArchiveServer(t)
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/archive/"+ts+"/output.html", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != "<html>hi</html>" {
		t.Errorf("body = %q, want archived html", rec.Body.String())
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Errorf("csp = %q, want sandbox", got)
	}
}

func TestHandleArchiveFile_traversalRejected(t *testing.T) {
	t.Parallel()
	s, ts := newArchiveServer(t)
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/archive/"+ts+"/../../../../../../etc/passwd", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (traversal blocked)", rec.Code, http.StatusNotFound)
	}
}

func TestHandleArchiveFile_directoryRejected(t *testing.T) {
	t.Parallel()
	s, ts := newArchiveServer(t)
	r := s.Router()

	// Request the snapshot dir itself (no file).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/archive/"+ts+"/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (directory listing blocked)", rec.Code, http.StatusNotFound)
	}
}

// TestHandleArchiveFile_indexHtmlNotRedirected verifies that requesting
// /archive/{timestamp}/index.html does NOT redirect to /archive/{timestamp}/
// (Go's http.ServeFile does this by default; we use ServeContent instead).
func TestHandleArchiveFile_indexHtmlNotRedirected(t *testing.T) {
	t.Parallel()
	s, ts := newArchiveServer(t)
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/archive/"+ts+"/index.html", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (no redirect)", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "<html>index</html>" {
		t.Errorf("body = %q, want index html", rec.Body.String())
	}
	// Ensure Location header is NOT set (no redirect)
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("Location header = %q, want empty (no redirect)", loc)
	}
}

func TestHandleArchiveFile_invalidTimestamp(t *testing.T) {
	t.Parallel()
	s, _ := newArchiveServer(t)
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/archive/not-a-ts/output.html", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleArchiveFile_missingFile(t *testing.T) {
	t.Parallel()
	s, ts := newArchiveServer(t)
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/archive/"+ts+"/nope.html", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
