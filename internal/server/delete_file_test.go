package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

const deleteFileTs int64 = 1700000000000000

// newDeleteFileSnapshot seeds a snapshot with a succeeded wget run producing
// output.html, plus an index.json on disk. It returns the server and the on-disk
// snapshot dir.
func newDeleteFileSnapshot(t *testing.T) (*Server, string) {
	t.Helper()
	db := newTestDB(t)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", deleteFileTs); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	snap, err := db.GetSnapshot(context.Background(), deleteFileTs)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	runID, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID,
		Extractor:  "wget",
		Status:     "succeeded",
		StartedAt:  deleteFileTs,
		FinishedAt: deleteFileTs + 1000,
	})
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if _, err := db.InsertStepOutput(context.Background(), runID, meta.StepOutput{
		RunID: runID, Name: "dom", Filename: "output.html", Status: "succeeded", StartTs: deleteFileTs, EndTs: deleteFileTs + 1000,
	}); err != nil {
		t.Fatalf("InsertStepOutput: %v", err)
	}

	root := filepath.Join(t.TempDir(), "archive")
	dir := archive.SnapshotDir(root, deleteFileTs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<html>hi</html>"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write output.html: %v", err)
	}
	// A pre-existing index.json so we can verify it is rebuilt after deletion.
	if err := os.WriteFile(filepath.Join(dir, archive.IndexFile), []byte(`{"latest":{"dom":"output.html"}}`), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write index.json: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	return s, dir
}

func deleteFileRequest(t *testing.T, s *Server, ts int64, filename string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	form := "filename=" + filename
	req := httptest.NewRequest(http.MethodPost, "/"+snapshot.Format(ts)+"/delete-file", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.Router().ServeHTTP(rec, req)
	return rec
}

func TestHandleDeleteFile_removesFileAndOutput(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)

	rec := deleteFileRequest(t, s, deleteFileTs, "output.html")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d (SeeOther)", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != snapshotPath(deleteFileTs) {
		t.Errorf("location = %q, want %q", loc, snapshotPath(deleteFileTs))
	}

	// The file is gone from disk.
	if _, err := os.Stat(filepath.Join(dir, "output.html")); !os.IsNotExist(err) {
		t.Errorf("output.html still exists on disk after delete")
	}

	// The step_outputs row is gone; index.json no longer references the file.
	snap, err := s.DB.GetSnapshot(context.Background(), deleteFileTs)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	runs, err := s.DB.ListRunsBySnapshot(context.Background(), snap.ID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != 1 || len(runs[0].Outputs) != 0 {
		t.Errorf("outputs after delete = %+v, want none", runs)
	}
	indexJSON, err := os.ReadFile(filepath.Join(dir, archive.IndexFile))
	if err != nil {
		t.Fatalf("read index.json: %v", err)
	}
	if strings.Contains(string(indexJSON), "output.html") {
		t.Errorf("index.json still references deleted file: %s", indexJSON)
	}
}

func TestHandleDeleteFile_nestedFile(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)
	// Add an unclaimed nested file (as old ArchiveBox imports have).
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatalf("MkdirAll media: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "video.mp4"), []byte("fakevideo"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	rec := deleteFileRequest(t, s, deleteFileTs, "media/video.mp4")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d (SeeOther)", rec.Code, http.StatusSeeOther)
	}
	if _, err := os.Stat(filepath.Join(dir, "media", "video.mp4")); !os.IsNotExist(err) {
		t.Errorf("media/video.mp4 still exists on disk after delete")
	}
}

func TestHandleDeleteFile_blockedWhilePending(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)
	// A pending rerun means the worker may start writing any moment.
	snap, err := s.DB.GetSnapshot(context.Background(), deleteFileTs)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if _, err := s.DB.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID, Extractor: "obelisk", Status: "pending",
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	rec := deleteFileRequest(t, s, deleteFileTs, "output.html")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (Conflict)", rec.Code, http.StatusConflict)
	}
	if _, err := os.Stat(filepath.Join(dir, "output.html")); err != nil {
		t.Errorf("output.html should survive a blocked delete: %v", err)
	}
}

func TestHandleDeleteFile_blockedWhileRunning(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)
	snap, err := s.DB.GetSnapshot(context.Background(), deleteFileTs)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if _, err := s.DB.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID, Extractor: "obelisk", Status: "running", StartedAt: deleteFileTs,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	rec := deleteFileRequest(t, s, deleteFileTs, "output.html")

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (Conflict)", rec.Code, http.StatusConflict)
	}
	if _, err := os.Stat(filepath.Join(dir, "output.html")); err != nil {
		t.Errorf("output.html should survive a blocked delete: %v", err)
	}
}

func TestHandleDeleteFile_indexJsonProtected(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)

	rec := deleteFileRequest(t, s, deleteFileTs, "index.json")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (BadRequest)", rec.Code, http.StatusBadRequest)
	}
	if _, err := os.Stat(filepath.Join(dir, archive.IndexFile)); err != nil {
		t.Errorf("index.json should never be deletable: %v", err)
	}
}

func TestHandleDeleteFile_traversalRejected(t *testing.T) {
	t.Parallel()
	s, _ := newDeleteFileSnapshot(t)

	rec := deleteFileRequest(t, s, deleteFileTs, "../../etc/passwd")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (BadRequest)", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteFile_missingFilename(t *testing.T) {
	t.Parallel()
	s, _ := newDeleteFileSnapshot(t)

	rec := deleteFileRequest(t, s, deleteFileTs, "")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (BadRequest)", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteFile_notFound(t *testing.T) {
	t.Parallel()
	s, _ := newDeleteFileSnapshot(t)

	rec := deleteFileRequest(t, s, 9999999999999999, "output.html")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (NotFound)", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDetail_showsDeleteButtonsWhenTerminal(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)
	// An unclaimed nested file gets a delete button too.
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0o755); err != nil {
		t.Fatalf("MkdirAll media: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "media", "video.mp4"), []byte("fakevideo"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write nested: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(deleteFileTs), nil)
	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "delete-file") {
		t.Errorf("detail body missing delete-file form")
	}
	if !strings.Contains(body, `value="output.html"`) {
		t.Errorf("detail body missing delete form for output.html")
	}
	if !strings.Contains(body, `value="media/video.mp4"`) {
		t.Errorf("detail body missing delete form for nested file")
	}
	if strings.Contains(body, `value="index.json"`) {
		t.Errorf("detail body must not offer to delete index.json")
	}
}

func TestHandleDetail_hidesDeleteButtonsWhileNonTerminal(t *testing.T) {
	t.Parallel()
	s, dir := newDeleteFileSnapshot(t)
	if err := os.WriteFile(filepath.Join(dir, "nested.txt"), []byte("x"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write nested: %v", err)
	}
	snap, err := s.DB.GetSnapshot(context.Background(), deleteFileTs)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if _, err := s.DB.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID, Extractor: "obelisk", Status: "running", StartedAt: deleteFileTs,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(deleteFileTs), nil)
	s.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), "delete-file") {
		t.Errorf("detail body shows delete buttons while a run is running")
	}
}
