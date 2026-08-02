package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// handleArchiveFile serves a single archived output file from the on-disk
// archive/{timestamp}/ tree under /archive/{timestamp}/…. It is path-scoped
// (the resolved file must stay within the snapshot's directory), and sets a
// sandbox CSP plus nosniff so archived HTML cannot touch the app or its
// cookies.
func (s *Server) handleArchiveFile(w http.ResponseWriter, r *http.Request) {
	timestampStr := chi.URLParam(r, "timestamp")
	timestamp, err := snapshot.Parse(timestampStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	subpath := chi.URLParam(r, "*")
	snapDir := archive.SnapshotDir(s.ArchiveRoot, timestamp)

	// Prepend "/" so filepath.Clean treats the subpath as absolute, neutralizing
	// any ".." segments before joining under the snapshot dir.
	full := filepath.Join(snapDir, filepath.Clean("/"+subpath))

	// Defense in depth: confirm the resolved path is still within snapDir.
	rel, err := filepath.Rel(snapDir, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		http.NotFound(w, r)
		return
	}

	// Refuse directory traversal/listing: only serve real, non-directory files.
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Use ServeContent instead of ServeFile to avoid Go's built-in redirect
	// of /index.html to / (see net/http/fs.go). ServeContent takes a filename
	// only for Content-Type detection, not for path-based redirects.
	f, err := os.Open(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	http.ServeContent(w, r, filepath.Base(full), info.ModTime(), f)
}
