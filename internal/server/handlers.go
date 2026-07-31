package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

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
