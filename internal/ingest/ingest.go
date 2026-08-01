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
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/headers"
	"github.com/nguyenmp/simplearchive/internal/extractors/obelisk"
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

// defaultPipeline is the ordered list of extractors run for every URL. The
// first extractor is the primary DOM fetch and is fatal: if it fails the ingest
// aborts. The remaining extractors are best-effort; their failures are logged
// at warn and do not fail the ingest.
func defaultPipeline() []extractors.Extractor {
	return []extractors.Extractor{
		wget.DOMExtractor{},
		wget.FaviconExtractor{},
		headers.Extractor{},
		obelisk.Extractor{},
	}
}

// Add archives a single URL inline. It creates a snapshot row, runs the
// extractor pipeline, writes the per-snapshot index.json, and marks the snapshot
// succeeded. The primary DOM fetch is fatal; favicon and headers failures are
// best-effort (logged at warn) and do not fail the ingest.
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

	pipeline := defaultPipeline()
	primary := pipeline[0]

	// Primary DOM fetch is fatal: on failure, record the run, mark the snapshot
	// failed, and abort (no best-effort extractors run, no index.json).
	pSteps, err := primary.Run(ctx, url, dir)
	recordRuns(ctx, db, resolved, pSteps)
	if err != nil {
		if ferr := db.MarkSnapshotFailed(ctx, resolved); ferr != nil {
			slog.Warn("ingest: mark snapshot failed", "err", ferr)
		}
		return Result{}, fmt.Errorf("ingest.Add: %s: %w", primary.Name(), err)
	}
	outputs := stepFilenames(pSteps)

	// Best-effort extractors.
	for _, ex := range pipeline[1:] {
		steps, runErr := ex.Run(ctx, url, dir)
		recordRuns(ctx, db, resolved, steps)
		if runErr != nil {
			slog.Warn("ingest: extractor", "extractor", ex.Name(), "url", url, "err", runErr)
		}
		outputs = append(outputs, stepFilenames(steps)...)
	}

	// Parse the title from the DOM output.
	title := ""
	if html, rerr := os.ReadFile(filepath.Join(dir, wget.OutputFile)); rerr == nil {
		title = archive.ParseTitle(html)
	} else {
		slog.Warn("ingest: read output.html", "err", rerr)
	}

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

// stepFilenames returns the output filename of each step, preserving order.
func stepFilenames(steps []extractors.Step) []string {
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Filename)
	}
	return out
}

// recordRuns persists one extractor_runs row per step. Failures here are logged
// at warn but never fail the ingest; the snapshot's own status is the source of
// truth for the overall outcome.
func recordRuns(ctx context.Context, db *meta.DB, ts int64, steps []extractors.Step) {
	for _, s := range steps {
		run := meta.ExtractorRun{
			Timestamp:  ts,
			Extractor:  s.Name,
			Status:     s.Status,
			Output:     s.Filename,
			StartedAt:  s.StartTs.UnixMicro(),
			FinishedAt: s.EndTs.UnixMicro(),
		}
		if s.Err != nil {
			run.Error = s.Err.Error()
		}
		if _, err := db.InsertRun(ctx, run); err != nil {
			slog.Warn("ingest: record run", "extractor", s.Name, "err", err)
		}
	}
}
