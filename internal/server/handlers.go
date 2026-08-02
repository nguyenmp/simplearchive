package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/ingest"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// addData is the view model for the Add-URL form.
type addData struct {
	URL   string
	Error string
}

const defaultPageSize = 50

// snapshotFileInfo wraps a snapshot with its on-disk file stats for the list view.
type snapshotFileInfo struct {
	meta.Snapshot
	FileCount int
	TotalSize int64
}

// listData is the view model for the list/search page.
type listData struct {
	Snapshots   []snapshotFileInfo
	Total       int
	Limit       int
	Offset      int
	Page        int // 1-indexed
	Pages       int
	HasPrev     bool
	HasNext     bool
	PrevOffset  int
	NextOffset  int
	Query       string // search query, empty when not searching
	IsSearch    bool   // true when showing search results
}

// handleList renders GET /: a paginated list of snapshots, or search results
// when the "q" query param is present.
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var data listData
	var err error

	if query != "" {
		data, err = s.searchSnapshotsData(r, query)
	} else {
		data, err = s.listSnapshotsData(r)
	}

	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := s.render.render(w, "list", data); err != nil {
		s.Logger.Error("list: render", "err", err)
	}
}

// listSnapshotsData handles the non-search list path: pagination parsing, DB
// query, and file stat computation.
func (s *Server) listSnapshotsData(r *http.Request) (listData, error) {
	limit := parsePositiveInt(r, "limit", defaultPageSize)
	offset := parsePositiveInt(r, "offset", 0)

	snaps, total, err := s.DB.ListSnapshots(r.Context(), limit, offset)
	if err != nil {
		s.Logger.Error("list: query", "limit", limit, "offset", offset, "err", err)
		return listData{}, err
	}

	infos := fileInfoForSnaps(s.ArchiveRoot, snaps)
	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	page := offset/limit + 1

	return listData{
		Snapshots:  infos,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Page:       page,
		Pages:      pages,
		HasPrev:    offset > 0,
		HasNext:    offset+limit < total,
		PrevOffset: max(offset-limit, 0),
		NextOffset: offset + limit,
	}, nil
}

// searchSnapshotsData handles the search path: search query execution, DB
// lookup, and file stat computation.
func (s *Server) searchSnapshotsData(r *http.Request, query string) (listData, error) {
	tsList, err := s.searchSnapshots(r.Context(), query)
	if err != nil {
		s.Logger.Error("list: searchSnapshots", "query", query, "err", err)
		return listData{}, err
	}

	var snaps []meta.Snapshot
	if len(tsList) > 0 {
		snaps, err = s.DB.GetSnapshotsByTimestamps(r.Context(), tsList)
		if err != nil {
			s.Logger.Error("list: GetSnapshotsByTimestamps", "query", query, "err", err)
			return listData{}, err
		}
	}

	infos := fileInfoForSnaps(s.ArchiveRoot, snaps)
	return listData{
		Snapshots: infos,
		Total:     len(snaps),
		Query:     query,
		IsSearch:  true,
	}, nil
}

// fileInfoForSnaps computes file counts and total sizes for each snapshot by
// walking its on-disk archive directory.
func fileInfoForSnaps(root string, snaps []meta.Snapshot) []snapshotFileInfo {
	infos := make([]snapshotFileInfo, 0, len(snaps))
	for _, snap := range snaps {
		info := snapshotFileInfo{Snapshot: snap}
		dir := archive.SnapshotDir(root, snap.Timestamp)
		_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			info.FileCount++
			if fi, fiErr := d.Info(); fiErr == nil {
				info.TotalSize += fi.Size()
			}
			return nil
		})
		infos = append(infos, info)
	}
	return infos
}

// detailData is the view model for the detail page.
type detailData struct {
	Snapshot            meta.Snapshot
	FileCount           int
	TotalSize           int64
	AvailableExtractors []string // names from the default pipeline (for re-run dropdown)
	// FilePaths maps an on-disk filename to its URL path, so extractor
	// outputs can link straight to the archived file.
	FilePaths map[string]string
	// FileSizes maps an on-disk filename to its size (bytes) for display.
	FileSizes map[string]int64
	// OtherFiles are on-disk files not claimed by any extractor output
	// (e.g. index.json).
	OtherFiles []fileLink
	Runs       []meta.ExtractorRun
}

// fileLink is a single archived output file linkable from the detail page.
type fileLink struct {
	Name string
	Path string // URL path under /archive/{timestamp}/
	Size int64  // bytes
}

// handleDetail renders GET /{timestamp}: a single snapshot's metadata plus
// links to its archived output files. The {timestamp} URL param is the
// ArchiveBox "seconds.microseconds" directory name.
func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	tsStr := chi.URLParam(r, "timestamp")
	ts, err := snapshot.Parse(tsStr)
	if err != nil {
		s.renderNotFound(w)
		return
	}

	snap, err := s.DB.GetSnapshot(r.Context(), ts)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			s.renderNotFound(w)
			return
		}
		s.Logger.Error("detail: query", "timestamp", tsStr, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var runs []meta.ExtractorRun
	if r, rerr := s.DB.ListRunsBySnapshot(r.Context(), snap.ID); rerr != nil {
		s.Logger.Error("detail: list runs", "err", rerr)
	} else {
		runs = r
	}

	if isAnyRunNonTerminal(runs) {
		w.Header().Set("Refresh", "1")
	}

	data, err := s.snapshotFiles(archive.SnapshotDir(s.ArchiveRoot, ts), ts, runs)
	if err != nil {
		s.Logger.Error("detail: snapshot files", "err", err)
	}
	data.Snapshot = snap
	data.AvailableExtractors = ingest.DefaultExtractorNames()
	data.Runs = runs

	if err := s.render.render(w, "detail", data); err != nil {
		s.Logger.Error("detail: render", "err", err)
	}
}

// isAnyRunNonTerminal returns true if any run is pending or running, which
// signals the detail page to auto-refresh.
func isAnyRunNonTerminal(runs []meta.ExtractorRun) bool {
	for _, run := range runs {
		if run.Status == extractors.StatusPending || run.Status == extractors.StatusRunning {
			return true
		}
	}
	return false
}

// snapshotFiles walks the snapshot directory and classifies files as claimed
// (produced by an extractor output) or unclaimed. It returns a detailData
// populated with FilePaths, FileSizes, FileCount, TotalSize, and OtherFiles.
func (s *Server) snapshotFiles(dir string, ts int64, runs []meta.ExtractorRun) (detailData, error) {
	claimed := make(map[string]bool)
	for _, run := range runs {
		for _, out := range run.Outputs {
			claimed[out.Filename] = true
		}
	}

	var data detailData
	data.FilePaths = make(map[string]string)
	data.FileSizes = make(map[string]int64)
	if err := filepath.WalkDir(dir, func(full string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, full)
		urlPath := "/archive/" + snapshot.Format(ts) + "/" + rel
		basename := filepath.Base(rel)
		data.FilePaths[basename] = urlPath
		data.FilePaths[rel] = urlPath
		var size int64
		if fi, err := d.Info(); err == nil {
			size = fi.Size()
			data.TotalSize += size
			data.FileSizes[basename] = size
			data.FileSizes[rel] = size
		}
		if !claimed[basename] {
			data.OtherFiles = append(data.OtherFiles, fileLink{Name: rel, Path: urlPath, Size: size})
		}
		data.FileCount++
		return nil
	}); err != nil {
		return data, err
	}
	return data, nil
}

// handleRerun accepts POST /{timestamp}/rerun: it inserts a new pending
// extractor_runs row for the chosen extractor on an existing snapshot so the
// worker will re-run it. The extractor name is validated against the default
// pipeline.
func (s *Server) handleRerun(w http.ResponseWriter, r *http.Request) {
	tsStr := chi.URLParam(r, "timestamp")
	ts, err := snapshot.Parse(tsStr)
	if err != nil {
		s.renderNotFound(w)
		return
	}

	snap, err := s.DB.GetSnapshot(r.Context(), ts)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			s.renderNotFound(w)
			return
		}
		s.Logger.Error("rerun: query", "timestamp", tsStr, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	extractor := r.FormValue("extractor")
	valid := false
	for _, name := range ingest.DefaultExtractorNames() {
		if name == extractor {
			valid = true
			break
		}
	}
	if !valid {
		http.Error(w, "unknown extractor", http.StatusBadRequest)
		return
	}

	if err := s.DB.InsertPendingRuns(r.Context(), snap.ID, []string{extractor}); err != nil {
		s.Logger.Error("rerun: insert pending run", "timestamp", tsStr, "extractor", extractor, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, snapshotPath(ts), http.StatusSeeOther)
}

// handleDelete accepts POST /{timestamp}/delete: it removes the snapshot from
// the database (ON DELETE CASCADE cleans up runs and outputs) and deletes its
// on-disk archive directory, then redirects to the list page.
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	tsStr := chi.URLParam(r, "timestamp")
	ts, err := snapshot.Parse(tsStr)
	if err != nil {
		s.renderNotFound(w)
		return
	}

	if err := s.DB.DeleteSnapshot(r.Context(), ts); err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			s.renderNotFound(w)
			return
		}
		s.Logger.Error("delete: db", "timestamp", tsStr, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := archive.RemoveSnapshot(s.ArchiveRoot, ts); err != nil {
		s.Logger.Error("delete: archive", "err", err)
		// Do not fail the request; the DB record is already gone.
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// renderNotFound renders the 404 page with the correct status code.
func (s *Server) renderNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	if err := s.render.render(w, "notfound", nil); err != nil {
		s.Logger.Error("notfound: render", "err", err)
	}
}

// handleAddForm renders GET /add: the Add-URL form.
func (s *Server) handleAddForm(w http.ResponseWriter, r *http.Request) {
	if err := s.render.render(w, "add", addData{}); err != nil {
		s.Logger.Error("add: render", "err", err)
	}
}

// handleAddSubmit handles POST /add: it enqueues the URL for archiving and
// immediately redirects to the new snapshot's detail page. The serve worker
// goroutine drains the pending extractor_runs asynchronously; the detail page
// shows the steps transitioning pending -> running -> terminal as it does.
func (s *Server) handleAddSubmit(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	if url == "" {
		s.renderAddError(w, url, "URL is required")
		return
	}

	_, ts, err := ingest.Enqueue(r.Context(), s.DB, url)
	if err != nil {
		s.Logger.Error("add: enqueue", "url", url, "err", err)
		s.renderAddError(w, url, "failed to enqueue: "+err.Error())
		return
	}

	http.Redirect(w, r, snapshotPath(ts), http.StatusSeeOther)
}

func (s *Server) renderAddError(w http.ResponseWriter, url, msg string) {
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := s.render.render(w, "add", addData{URL: url, Error: msg}); err != nil {
		s.Logger.Error("add: render error", "err", err)
	}
}

// formatTimestamp renders an epoch-microsecond timestamp as a human-readable
// local time for display.
func formatTimestamp(ts int64) string {
	return time.UnixMicro(ts).Format("2006-01-02 15:04:05")
}

// snapshotPath returns the URL path for a snapshot's detail page, using the
// ArchiveBox "seconds.microseconds" directory name.
func snapshotPath(ts int64) string {
	return "/" + snapshot.Format(ts)
}

func parsePositiveInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// searchSnapshots shells out to ripgrep to find archive files containing q.
// It returns a deduplicated, newest-first list of snapshot timestamps.
// An empty q returns nil. If ripgrep exits with an unexpected code, an error
// is returned.
func (s *Server) searchSnapshots(ctx context.Context, q string) ([]int64, error) {
	if q == "" {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "rg",
		"-l", "--no-ignore", "--ignore-case",
		"--", q, s.ArchiveRoot,
	)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// No matches.
			return nil, nil
		}
		return nil, fmt.Errorf("rg: %w", err)
	}

	seen := make(map[int64]struct{})
	var results []int64
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(s.ArchiveRoot, line)
		if err != nil {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 0 {
			continue
		}
		tsStr := parts[0]
		ts, err := snapshot.Parse(tsStr)
		if err != nil {
			continue
		}
		if _, ok := seen[ts]; ok {
			continue
		}
		seen[ts] = struct{}{}
		results = append(results, ts)
	}

	// ripgrep walks in filesystem order; sort newest-first for the UI.
	slices.SortFunc(results, func(a, b int64) int {
		if a > b {
			return -1
		}
		if a < b {
			return 1
		}
		return 0
	})

	return results, nil
}
