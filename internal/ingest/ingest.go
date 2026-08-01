// Package ingest runs the inline archiving pipeline for a single URL: create a
// snapshot row, fetch the page (wget) plus favicon and headers, write the
// per-snapshot index.json, and mark the snapshot succeeded. It is the shared
// core used by both the 'add' CLI command and the web Add-URL form.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedp"
	"github.com/nguyenmp/simplearchive/internal/extractors/headers"
	"github.com/nguyenmp/simplearchive/internal/extractors/obelisk"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
	"github.com/nguyenmp/simplearchive/internal/extractors/ytdlp"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// Result is the outcome of a successful Add.
type Result struct {
	SnapshotID int64
	Timestamp  int64
	Title      string
	Dir        string // on-disk snapshot directory
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
		ytdlp.Extractor{},
		chromedp.Extractor{},
	}
}

// Add archives a single URL inline. It creates a snapshot row, runs the
// extractor pipeline, writes the per-snapshot index.json, and marks the snapshot
// succeeded. The primary DOM fetch is fatal; favicon and headers failures are
// best-effort (logged at warn) and do not fail the ingest.
func Add(ctx context.Context, db *meta.DB, archiveRoot, url string) (Result, error) {
	ts := snapshot.NewTimestamp()
	snapshotID, err := db.CreateSnapshot(ctx, url, ts)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.Add: create snapshot: %w", err)
	}

	dir, err := archive.MkdirSnapshot(archiveRoot, ts)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.Add: mkdir: %w", err)
	}

	pipeline := defaultPipeline()
	primary := pipeline[0]

	// Primary DOM fetch is fatal: on failure, record the run and abort (no
	// best-effort extractors run, no index.json). The snapshot has no stored
	// status; its failed state is derived from the failed wget run.
	pSteps, err := primary.Run(ctx, url, dir)
	recordRuns(ctx, db, snapshotID, primary.Name(), pSteps)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.Add: %s: %w", primary.Name(), err)
	}
	steps := append([]extractors.Step(nil), pSteps...)

	// Best-effort extractors. Skipped extractors (ErrSkipped) contribute no steps
	// and are not warned about; other failures are logged at warn.
	for _, ex := range pipeline[1:] {
		es, runErr := ex.Run(ctx, url, dir)
		recordRuns(ctx, db, snapshotID, ex.Name(), es)
		if runErr != nil && !errors.Is(runErr, extractors.ErrSkipped) {
			slog.Warn("ingest: extractor", "extractor", ex.Name(), "url", url, "err", runErr)
		}
		steps = append(steps, es...)
	}

	// Parse the title from the DOM output.
	title := ""
	if html, rerr := os.ReadFile(filepath.Join(dir, wget.OutputFile)); rerr == nil {
		title = archive.ParseTitle(html)
	} else {
		slog.Warn("ingest: read output.html", "err", rerr)
	}

	if err := archive.WriteIndex(archive.IndexData{
		Timestamp: ts,
		URL:       url,
		Title:     title,
		Dir:       dir,
		Steps:     steps,
	}); err != nil {
		return Result{}, fmt.Errorf("ingest.Add: write index: %w", err)
	}

	if err := db.UpdateSnapshot(ctx, ts, title); err != nil {
		return Result{}, fmt.Errorf("ingest.Add: update snapshot: %w", err)
	}

	return Result{SnapshotID: snapshotID, Timestamp: ts, Title: title, Dir: dir}, nil
}

// recordRuns persists one extractor_runs row (per extractor) plus one
// step_outputs row per output the extractor produced. A skipped extractor that
// produced no steps records nothing, preserving the prior behavior. Failures
// here are logged at warn but never fail the ingest; the snapshot's own status
// is the source of truth for the overall outcome.
func recordRuns(ctx context.Context, db *meta.DB, snapshotID int64, extractor string, steps []extractors.Step) {
	if len(steps) == 0 {
		return
	}
	run := meta.ExtractorRun{
		SnapshotID: snapshotID,
		Extractor:  extractor,
		Status:     aggregateRunStatus(steps),
		StartedAt:  minStepMicros(steps, true),
		FinishedAt: minStepMicros(steps, false),
	}
	for _, s := range steps {
		if s.Err != nil {
			run.Error = s.Err.Error()
			break
		}
	}
	runID, err := db.InsertRun(ctx, run)
	if err != nil {
		slog.Warn("ingest: record run", "extractor", extractor, "err", err)
		return
	}
	for _, s := range steps {
		out := meta.StepOutput{
			RunID:    runID,
			Name:     s.Name,
			Filename: s.Filename,
			Cmd:      s.Cmd,
			Status:   s.Status,
			StartTs:  s.StartTs.UnixMicro(),
			EndTs:    s.EndTs.UnixMicro(),
		}
		if s.Err != nil {
			out.Error = s.Err.Error()
		}
		if _, err := db.InsertStepOutput(ctx, runID, out); err != nil {
			slog.Warn("ingest: record step output", "extractor", extractor, "step", s.Name, "err", err)
		}
	}
}

// aggregateRunStatus derives an extractor run's status from its steps: failed
// if any step failed, succeeded if any succeeded, else skipped.
func aggregateRunStatus(steps []extractors.Step) string {
	var anySucceeded, anyFailed bool
	for _, s := range steps {
		switch s.Status {
		case extractors.StatusFailed:
			anyFailed = true
		case extractors.StatusSucceeded:
			anySucceeded = true
		}
	}
	if anyFailed {
		return extractors.StatusFailed
	}
	if anySucceeded {
		return extractors.StatusSucceeded
	}
	return extractors.StatusSkipped
}

// minStepMicros returns the earliest step start (when wantStart) or the latest
// step end (otherwise), in epoch microseconds. Steps with a zero time are
// ignored so a missing timestamp does not collapse the range to 0.
func minStepMicros(steps []extractors.Step, wantStart bool) int64 {
	var out int64
	for _, s := range steps {
		var v int64
		if wantStart {
			v = s.StartTs.UnixMicro()
		} else {
			v = s.EndTs.UnixMicro()
		}
		if v == 0 {
			continue
		}
		if out == 0 || (wantStart && v < out) || (!wantStart && v > out) {
			out = v
		}
	}
	return out
}
