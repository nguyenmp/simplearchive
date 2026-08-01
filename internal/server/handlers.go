package server

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nguyenmp/simplearchive/internal/archive"
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

// listData is the view model for the list page.
type listData struct {
	Snapshots   []meta.Snapshot
	Total       int
	Limit       int
	Offset      int
	Page        int // 1-indexed
	Pages       int
	HasPrev     bool
	HasNext     bool
	PrevOffset  int
	NextOffset  int
}

// handleList renders GET /: a paginated, newest-first table of snapshots.
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	limit := parsePositiveInt(r, "limit", defaultPageSize)
	offset := parsePositiveInt(r, "offset", 0)

	snaps, total, err := s.DB.ListSnapshots(r.Context(), limit, offset)
	if err != nil {
		s.Logger.Error("list: query", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	pages := 0
	if total > 0 {
		pages = (total + limit - 1) / limit
	}
	page := offset/limit + 1

	data := listData{
		Snapshots:  snaps,
		Total:      total,
		Limit:      limit,
		Offset:     offset,
		Page:       page,
		Pages:      pages,
		HasPrev:    offset > 0,
		HasNext:    offset+limit < total,
		PrevOffset: max(offset-limit, 0),
		NextOffset: offset + limit,
	}
	if err := s.render.render(w, "list", data); err != nil {
		s.Logger.Error("list: render", "err", err)
	}
}

// detailData is the view model for the detail page.
type detailData struct {
	Snapshot meta.Snapshot
	Files    []fileLink
	Runs     []meta.ExtractorRun
}

// fileLink is a single archived output file linkable from the detail page.
type fileLink struct {
	Name string
	Path string // URL path under /archive/{timestamp}/
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
		s.Logger.Error("detail: query", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	data := detailData{Snapshot: snap}
	if runs, rerr := s.DB.ListRunsByTimestamp(r.Context(), ts); rerr != nil {
		s.Logger.Error("detail: list runs", "err", rerr)
	} else {
		data.Runs = runs
	}
	dir := archive.SnapshotDir(s.ArchiveRoot, ts)
	if entries, derr := os.ReadDir(dir); derr == nil {
		formatted := snapshot.Format(ts)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			data.Files = append(data.Files, fileLink{
				Name: name,
				Path: "/archive/" + formatted + "/" + name,
			})
		}
	}

	if err := s.render.render(w, "detail", data); err != nil {
		s.Logger.Error("detail: render", "err", err)
	}
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

// handleAddSubmit handles POST /add: it archives the URL synchronously (inline
// wget pipeline) and redirects to the new snapshot's detail page. On error the
// form is re-rendered with the error message.
func (s *Server) handleAddSubmit(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	if url == "" {
		s.renderAddError(w, url, "URL is required")
		return
	}

	res, err := ingest.Add(r.Context(), s.DB, s.ArchiveRoot, url)
	if err != nil {
		s.Logger.Error("add: ingest", "url", url, "err", err)
		s.renderAddError(w, url, "failed to archive: "+err.Error())
		return
	}

	http.Redirect(w, r, snapshotPath(res.Timestamp), http.StatusSeeOther)
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
