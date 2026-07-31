// Package ingest runs the inline archiving pipeline for a single URL: create a
// snapshot row, fetch the page (wget) plus favicon and headers, write the
// per-snapshot index.json, and mark the snapshot succeeded. It is the shared
// core used by both the 'add' CLI command and the web Add-URL form.
package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors/headers"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// Result is the outcome of a successful Add.
type Result struct {
	Timestamp int64
	Title     string
	Dir       string // on-disk snapshot directory
}

// Add archives a single URL inline. It creates a snapshot row, fetches the page
// (wget) plus favicon and headers, writes the per-snapshot index.json, and
// marks the snapshot succeeded. Favicon and headers failures are best-effort
// (logged at warn) and do not fail the ingest.
func Add(ctx context.Context, db *meta.DB, archiveRoot, url string) (Result, error) {
	ts := snapshot.NewTimestamp()
	resolved, err := db.CreateSnapshot(ctx, url, ts)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.Add: create snapshot: %w", err)
	}

	dir, err := archive.MkdirSnapshot(archiveRoot, resolved)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.Add: mkdir: %w", err)
	}

	htmlPath, err := wget.Fetch(ctx, url, dir)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.Add: wget: %w", err)
	}
	if _, err := wget.FetchFavicon(ctx, url, dir); err != nil {
		slog.Warn("ingest: favicon", "url", url, "err", err)
	}
	if _, err := headers.Fetch(ctx, url, dir); err != nil {
		slog.Warn("ingest: headers", "url", url, "err", err)
	}

	title := ""
	if html, rerr := os.ReadFile(htmlPath); rerr == nil {
		title = archive.ParseTitle(html)
	} else {
		slog.Warn("ingest: read output.html", "err", rerr)
	}

	outputs := []string{filepath.Base(htmlPath), wget.FaviconFile, headers.OutputFile}
	if err := archive.WriteIndex(archive.IndexData{
		Timestamp: resolved,
		URL:       url,
		Title:     title,
		Dir:       dir,
		Outputs:   outputs,
	}); err != nil {
		return Result{}, fmt.Errorf("ingest.Add: write index: %w", err)
	}

	if err := db.UpdateSnapshot(ctx, resolved, title); err != nil {
		return Result{}, fmt.Errorf("ingest.Add: update snapshot: %w", err)
	}

	return Result{Timestamp: resolved, Title: title, Dir: dir}, nil
}
