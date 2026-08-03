package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// seedSnapshots inserts n succeeded snapshots into db for view tests.
func seedSnapshots(t *testing.T, db *meta.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		timestamp := int64(1700000000000000 + i)
		if _, err := db.CreateSnapshot(context.Background(), "https://example.com/"+strconv.Itoa(i), timestamp); err != nil {
			t.Fatalf("CreateSnapshot: %v", err)
		}
		if err := db.UpdateSnapshot(context.Background(), timestamp, "Title "+strconv.Itoa(i)); err != nil {
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

func TestHandleList_paginationDegenerateLimit(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 3)
	s := &Server{DB: db}
	r := s.Router()

	// ?limit=0 previously divided by zero in pagination math; it must fall
	// back to the default page size and render a normal page instead of
	// panicking.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?limit=0", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (limit=0 must not panic)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "3 total") {
		t.Errorf("body missing rendered list for defaulted limit: %q", rec.Body.String())
	}
}

func TestHandleDetail_found(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	const timestamp int64 = 1700000000000000

	// Create a snapshot dir with one output file so the file list is populated.
	root := filepath.Join(t.TempDir(), "archive")
	dir := archive.SnapshotDir(root, timestamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte("<html>hi</html>"), extractors.DefaultFilePerm); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Seed an extractor run so the per-extractor status table is populated.
	snap, err := db.GetSnapshot(context.Background(), timestamp)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	runID, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID,
		Extractor:  "wget",
		Status:     "succeeded",
		StartedAt:  timestamp,
		FinishedAt: timestamp + 1000,
	})
	if err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if _, err := db.InsertStepOutput(context.Background(), runID, meta.StepOutput{
		RunID: runID, Name: "dom", Filename: "output.html", Status: "succeeded", StartTs: timestamp, EndTs: timestamp + 1000,
	}); err != nil {
		t.Fatalf("InsertStepOutput: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
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
	if !strings.Contains(body, "/archive/"+snapshot.Format(timestamp)+"/output.html") {
		t.Errorf("body missing archive file path: %q", body)
	}
	// Per-extractor status table is rendered: the wget run and its dom output.
	if !strings.Contains(body, "Extractors") {
		t.Errorf("body missing extractors heading: %q", body)
	}
	if !strings.Contains(body, "wget") {
		t.Errorf("body missing wget extractor row: %q", body)
	}
	if !strings.Contains(body, "dom") || !strings.Contains(body, "succeeded") {
		t.Errorf("body missing dom output / succeeded status: %q", body)
	}
}

func TestHandleDetail_showsRunDuration(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	const timestamp int64 = 1700000000000000

	snap, err := db.GetSnapshot(context.Background(), timestamp)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	// A finished 14-second run shows its duration; a still-running run does not.
	if _, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID,
		Extractor:  "wget",
		Status:     "succeeded",
		StartedAt:  timestamp,
		FinishedAt: timestamp + 14_000_000,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}
	if _, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID,
		Extractor:  "obelisk",
		Status:     "running",
		StartedAt:  timestamp,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: filepath.Join(t.TempDir(), "archive")}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "14s") {
		t.Errorf("body missing run duration 14s: %q", body)
	}
	if !strings.Contains(body, `title="started `+formatTimestamp(timestamp)) {
		t.Errorf("body missing start-time tooltip: %q", body)
	}
	// The running (unfinished) obelisk run must not show a duration: only one
	// duration tooltip should be rendered in total.
	if n := strings.Count(body, `title="started `); n != 1 {
		t.Errorf("duration tooltip count = %d, want 1 (unfinished runs show no duration): %q", n, body)
	}
}

func TestHandleDetail_autoRefresh_pending(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	const timestamp int64 = 1700000000000000

	snap, err := db.GetSnapshot(context.Background(), timestamp)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if _, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID,
		Extractor:  "wget",
		Status:     "pending",
		StartedAt:  timestamp,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: filepath.Join(t.TempDir(), "archive")}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Refresh"); got != "1" {
		t.Errorf("Refresh header = %q, want %q (pending run should auto-refresh)", got, "1")
	}
}

func TestHandleDetail_autoRefresh_running(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	const timestamp int64 = 1700000000000000

	snap, err := db.GetSnapshot(context.Background(), timestamp)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if _, err := db.InsertRun(context.Background(), meta.ExtractorRun{
		SnapshotID: snap.ID,
		Extractor:  "wget",
		Status:     "running",
		StartedAt:  timestamp,
	}); err != nil {
		t.Fatalf("InsertRun: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: filepath.Join(t.TempDir(), "archive")}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Refresh"); got != "1" {
		t.Errorf("Refresh header = %q, want %q (running run should auto-refresh)", got, "1")
	}
}

func TestHandleDetail_noAutoRefreshWhenTerminal(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	const timestamp int64 = 1700000000000000

	snap, err := db.GetSnapshot(context.Background(), timestamp)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	for _, status := range []string{"succeeded", "failed", "skipped"} {
		if _, err := db.InsertRun(context.Background(), meta.ExtractorRun{
			SnapshotID: snap.ID,
			Extractor:  "wget",
			Status:     status,
			StartedAt:  timestamp,
			FinishedAt: timestamp + 1000,
		}); err != nil {
			t.Fatalf("InsertRun(%s): %v", status, err)
		}
	}

	s := &Server{DB: db, ArchiveRoot: filepath.Join(t.TempDir(), "archive")}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Refresh"); got != "" {
		t.Errorf("Refresh header = %q, want empty (all runs terminal)", got)
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
	snaps, _, err := db.ListSnapshots(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(snaps) != 1 {
		t.Errorf("snapshot count = %d, want 1", len(snaps))
	}
	if len(snaps) > 0 && snaps[0].URL != upstream.URL {
		t.Errorf("url = %q, want %q", snaps[0].URL, upstream.URL)
	}
}

func TestHandleList_withQuery_searches(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed")
	}

	db := newTestDB(t)
	seedSnapshots(t, db, 3)

	root := filepath.Join(t.TempDir(), "archive")
	// Snapshot 0 has a matching file; snapshot 1 does not.
	for i := 0; i < 3; i++ {
		timestamp := int64(1700000000000000 + i)
		dir := archive.SnapshotDir(root, timestamp)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		content := fmt.Sprintf("content for snapshot %d", i)
		if i == 0 {
			content = "unique keyword matchme here"
		}
		if err := os.WriteFile(filepath.Join(dir, "output.html"), []byte(content), extractors.DefaultFilePerm); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?q=matchme", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusOK, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Search") {
		t.Errorf("body missing search header: %q", body)
	}
	if !strings.Contains(body, "matchme") {
		t.Errorf("body missing query term: %q", body)
	}
	if !strings.Contains(body, "Title 0") {
		t.Errorf("body missing matching snapshot title: %q", body)
	}
	if strings.Contains(body, "Title 1") {
		t.Errorf("body should not contain non-matching snapshot: %q", body)
	}
}

func TestHandleList_withQuery_noMatches(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep not installed")
	}

	db := newTestDB(t)
	seedSnapshots(t, db, 1)
	root := filepath.Join(t.TempDir(), "archive")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: root}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?q=xyznotfound", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "No snapshots found") {
		t.Errorf("body missing no-results message: %q", rec.Body.String())
	}
}

func TestHandleRerun_enqueuesPendingRun(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	s := &Server{DB: db, ArchiveRoot: filepath.Join(t.TempDir(), "archive")}
	r := s.Router()

	form := "extractor=wget"
	req := httptest.NewRequest(http.MethodPost, "/"+	snapshot.Format(timestamp)+"/rerun", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body=%q", rec.Code, http.StatusSeeOther, rec.Body.String())
	}

	snap, err := db.GetSnapshot(context.Background(), timestamp)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	runs, err := db.ListRunsBySnapshot(context.Background(), snap.ID)
	if err != nil {
		t.Fatalf("ListRunsBySnapshot: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Extractor != "wget" || runs[0].Status != "pending" {
		t.Errorf("run = %+v, want pending wget", runs[0])
	}
}

func TestHandleRerun_unknownExtractor(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	s := &Server{DB: db}
	r := s.Router()

	form := "extractor=not-an-extractor"
	req := httptest.NewRequest(http.MethodPost, "/"+	snapshot.Format(timestamp)+"/rerun", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRerun_notFound(t *testing.T) {
	t.Parallel()
	s := &Server{DB: newTestDB(t)}
	r := s.Router()

	form := "extractor=wget"
	req := httptest.NewRequest(http.MethodPost, "/9999999999999999/rerun", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDetail_hasResubmitAndRerun(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	timestamp := int64(1700000000000000)
	if _, err := db.CreateSnapshot(context.Background(), "https://example.com", timestamp); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	s := &Server{DB: db}
	r := s.Router()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/"+snapshot.Format(timestamp), nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Resubmit as new snapshot") {
		t.Errorf("detail body missing resubmit button")
	}
	if !strings.Contains(body, "Re-run extractor") {
		t.Errorf("detail body missing rerun button")
	}
	if !strings.Contains(body, `value="wget"`) {
		t.Errorf("detail body missing wget option value in rerun dropdown")
	}
}
