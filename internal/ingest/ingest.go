// Package ingest runs the archiving pipeline for a single URL. The pipeline is
// modeled as a set of independent extractor steps persisted as pending
// extractor_runs rows (Enqueue), then drained by RunSnapshot, which runs each
// step, records its outputs, and rebuilds the per-snapshot index.json as each
// extractor finishes. Steps are independent: no step is fatal to the others,
// and a snapshot's state is derived from its steps (see the Deferred
// milestone), not stored on the snapshot.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

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

// Result is the outcome of archiving a snapshot. A snapshot has no stored
// status; per-step status lives in extractor_runs, so "success" is read from
// the steps (the wget/dom step in particular), not from Result.
type Result struct {
	SnapshotID int64
	Timestamp  int64
	Title      string
	Dir        string // on-disk snapshot directory
}

// defaultPipeline is the ordered list of extractors run for every URL. Steps
// are independent: no step is fatal to the others, and the order only affects
// display (insertion/id order) and that the DOM fetch runs first so the title
// is available for index.json.
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

// extractorByName maps an extractor's Name() to its instance so the worker can
// dispatch a pending extractor_runs row to the right extractor.
func extractorByName() map[string]extractors.Extractor {
	out := make(map[string]extractors.Extractor, len(defaultPipeline()))
	for _, ex := range defaultPipeline() {
		out[ex.Name()] = ex
	}
	return out
}

// Enqueue submits a URL for archiving: it creates a snapshot row and one
// "pending" extractor_runs row per extractor in the default pipeline, then
// returns the snapshot's id and timestamp. Archiving happens when RunSnapshot
// drains the snapshot (run inline by `add`, or by a worker goroutine in
// `serve`). The on-disk snapshot directory is created by RunSnapshot.
func Enqueue(ctx context.Context, db *meta.DB, url string) (int64, int64, error) {
	ts := snapshot.NewTimestamp()
	snapshotID, err := db.CreateSnapshot(ctx, url, ts)
	if err != nil {
		return 0, 0, fmt.Errorf("ingest.Enqueue: create snapshot: %w", err)
	}
	names := make([]string, 0, len(defaultPipeline()))
	for _, ex := range defaultPipeline() {
		names = append(names, ex.Name())
	}
	if err := db.InsertPendingRuns(ctx, snapshotID, names); err != nil {
		return 0, 0, fmt.Errorf("ingest.Enqueue: insert pending runs: %w", err)
	}
	return snapshotID, ts, nil
}

// RunSnapshot archives a snapshot: it claims the snapshot's pending runs
// (pending->running), runs each independently, records its outputs, rebuilds
// index.json as each extractor finishes, and records the title when the DOM
// fetch (wget) succeeds. If the snapshot's runs were already claimed by
// another caller (e.g. a serve worker), RunSnapshot waits for them to reach a
// terminal state and returns the recorded result. It does not fail the
// snapshot when a step fails — per-step status is the source of truth.
func RunSnapshot(ctx context.Context, db *meta.DB, archiveRoot string, snapshotID int64) (Result, error) {
	claimed, err := db.ClaimSnapshotRuns(ctx, snapshotID)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.RunSnapshot: claim: %w", err)
	}
	if claimed == 0 {
		// Another caller is already archiving this snapshot; wait for it.
		return waitForSnapshot(ctx, db, archiveRoot, snapshotID)
	}

	snap, err := db.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.RunSnapshot: get snapshot: %w", err)
	}
	dir, err := archive.MkdirSnapshot(archiveRoot, snap.Timestamp)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.RunSnapshot: mkdir: %w", err)
	}

	runs, err := db.ListRunsBySnapshot(ctx, snapshotID)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.RunSnapshot: list runs: %w", err)
	}
	registry := extractorByName()
	for i := range runs {
		r := &runs[i]
		if r.Status != extractors.StatusRunning {
			continue // already terminal (e.g. a prior partial run)
		}
		runOne(ctx, db, registry, r, snap, dir)
		rebuildIndex(ctx, db, dir, snap)
	}

	title := parseTitle(dir)
	if title != "" {
		if err := db.UpdateSnapshot(ctx, snap.Timestamp, title); err != nil {
			slog.Warn("ingest: update title", "err", err)
		}
		snap.Title = title
		rebuildIndex(ctx, db, dir, snap)
	}
	return Result{SnapshotID: snapshotID, Timestamp: snap.Timestamp, Title: snap.Title, Dir: dir}, nil
}

// runOne executes a single extractor run, records its outputs, and marks the
// run terminal. A skipped extractor (ErrSkipped) records a "skipped" run with
// no outputs; other failures record a "failed" run with the error. Failures
// are logged at warn but never abort the snapshot.
func runOne(ctx context.Context, db *meta.DB, registry map[string]extractors.Extractor, r *meta.ExtractorRun, snap meta.Snapshot, dir string) {
	ex, ok := registry[r.Extractor]
	if !ok {
		slog.Warn("ingest: no extractor registered", "extractor", r.Extractor)
		_ = db.FinishRun(ctx, r.ID, extractors.StatusFailed, nowMicros(), "no extractor registered for "+r.Extractor)
		return
	}
	steps, runErr := ex.Run(ctx, snap.URL, dir)
	if runErr != nil && !errors.Is(runErr, extractors.ErrSkipped) {
		slog.Warn("ingest: extractor", "extractor", ex.Name(), "url", snap.URL, "err", runErr)
	}

	status := aggregateRunStatus(steps)
	errMsg := ""
	if runErr != nil && !errors.Is(runErr, extractors.ErrSkipped) {
		status = extractors.StatusFailed
		errMsg = runErr.Error()
	}
	if errors.Is(runErr, extractors.ErrSkipped) {
		status = extractors.StatusSkipped
	}

	if err := db.DeleteStepOutputs(ctx, r.ID); err != nil {
		slog.Warn("ingest: clear step outputs", "extractor", ex.Name(), "err", err)
	}
	for _, s := range steps {
		out := meta.StepOutput{
			RunID:    r.ID,
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
		if _, err := db.InsertStepOutput(ctx, r.ID, out); err != nil {
			slog.Warn("ingest: record step output", "extractor", ex.Name(), "step", s.Name, "err", err)
		}
	}
	if err := db.FinishRun(ctx, r.ID, status, nowMicros(), errMsg); err != nil {
		slog.Warn("ingest: finish run", "extractor", ex.Name(), "err", err)
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

// rebuildIndex rewrites the per-snapshot index.json as a projection of the
// snapshot's terminal extractor_runs + step_outputs. It is called after each
// extractor finishes so the on-disk index reflects durable DB state (crash-safe
// and resumable).
func rebuildIndex(ctx context.Context, db *meta.DB, dir string, snap meta.Snapshot) {
	runs, err := db.ListRunsBySnapshot(ctx, snap.ID)
	if err != nil {
		slog.Warn("ingest: list runs for index", "err", err)
		return
	}
	steps := runsToSteps(runs)
	if err := archive.WriteIndex(archive.IndexData{
		Timestamp: snap.Timestamp,
		URL:       snap.URL,
		Title:     snap.Title,
		Dir:       dir,
		Steps:     steps,
	}); err != nil {
		slog.Warn("ingest: rebuild index.json", "err", err)
	}
}

// runsToSteps flattens a snapshot's terminal runs' outputs into the
// extractors.Step list archive.WriteIndex expects. Skipped/failed runs with no
// outputs contribute nothing; the per-output status is what index.json records.
func runsToSteps(runs []meta.ExtractorRun) []extractors.Step {
	out := make([]extractors.Step, 0)
	for _, r := range runs {
		for _, o := range r.Outputs {
			out = append(out, extractors.Step{
				Name:     o.Name,
				Filename: o.Filename,
				Cmd:      o.Cmd,
				Status:   o.Status,
			})
		}
	}
	return out
}

// parseTitle reads the DOM output and extracts the page title. Empty when the
// DOM file is absent or has no title.
func parseTitle(dir string) string {
	html, err := os.ReadFile(filepath.Join(dir, wget.OutputFile))
	if err != nil {
		return ""
	}
	return archive.ParseTitle(html)
}

// waitForSnapshot polls until the given snapshot's runs are all terminal (another
// caller is archiving it), then returns the recorded result. It is the
// fallback when RunSnapshot is asked to run a snapshot it did not claim.
func waitForSnapshot(ctx context.Context, db *meta.DB, archiveRoot string, snapshotID int64) (Result, error) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		runs, err := db.ListRunsBySnapshot(ctx, snapshotID)
		if err != nil {
			return Result{}, fmt.Errorf("ingest.waitForSnapshot: list runs: %w", err)
		}
		if allTerminal(runs) {
			snap, err := db.GetSnapshotByID(ctx, snapshotID)
			if err != nil {
				return Result{}, fmt.Errorf("ingest.waitForSnapshot: get snapshot: %w", err)
			}
			return Result{SnapshotID: snapshotID, Timestamp: snap.Timestamp, Title: snap.Title, Dir: archive.SnapshotDir(archiveRoot, snap.Timestamp)}, nil
		}
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func allTerminal(runs []meta.ExtractorRun) bool {
	if len(runs) == 0 {
		return false
	}
	for _, r := range runs {
		if r.Status == extractors.StatusPending || r.Status == extractors.StatusRunning {
			return false
		}
	}
	return true
}

func nowMicros() int64 { return time.Now().UnixMicro() }

// Add is the inline convenience used by the `add` CLI: it enqueues a URL and
// runs its snapshot synchronously, returning the result. It blocks until the
// snapshot is archived (by this call or, if one is running, by a serve worker).
func Add(ctx context.Context, db *meta.DB, archiveRoot, url string) (Result, error) {
	snapshotID, _, err := Enqueue(ctx, db, url)
	if err != nil {
		return Result{}, err
	}
	return RunSnapshot(ctx, db, archiveRoot, snapshotID)
}
