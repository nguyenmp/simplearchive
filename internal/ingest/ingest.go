// Package ingest runs the archiving pipeline for a single URL. The pipeline is
// modeled as a set of independent extractor steps persisted as pending
// extractor_runs rows (Enqueue), then drained by the serve worker (RunNext),
// which runs each step, records its outputs, and rebuilds the per-snapshot
// index.json as each extractor finishes. Steps are independent: no step is
// fatal to the others, and a snapshot's state is derived from its steps (see
// the Deferred milestone), not stored on the snapshot.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
// display (insertion/id order). The DOM fetch runs first so that its title is
// available for index.json as soon as the first run finishes.
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

// RunNext claims and archives one waiting snapshot (the worker loop's unit).
// It returns the archived Result and ran=true when it archived a snapshot, or
// a zero Result and ran=false when no snapshot was waiting. The caller loops,
// sleeping briefly between false results.
func RunNext(ctx context.Context, db *meta.DB, archiveRoot string) (Result, bool, error) {
	snapshotID, ok, err := db.ClaimNextSnapshot(ctx)
	if err != nil {
		return Result{}, false, fmt.Errorf("ingest.RunNext: claim: %w", err)
	}
	if !ok {
		return Result{}, false, nil
	}
	res, err := runClaimedSnapshot(ctx, db, archiveRoot, snapshotID)
	if err != nil {
		return Result{}, true, fmt.Errorf("ingest.RunNext: run: %w", err)
	}
	return res, true, nil
}

// runClaimedSnapshot runs a snapshot whose oldest pending run has been claimed
// (status "running") by ClaimNextSnapshot: it executes each non-terminal run in
// id order, starting a pending run (pending -> running, started_at stamped) the
// instant before it runs so that "running" tracks the extractor currently
// executing rather than every run claimed for the snapshot. After each run it
// parses the best available title, persists it, and rebuilds index.json before
// marking the run done — so index.json always reflects the snapshot's current
// state (including title) under the snapshot lock. It is the shared core used
// by RunNext (the serve worker).
func runClaimedSnapshot(ctx context.Context, db *meta.DB, archiveRoot string, snapshotID int64) (Result, error) {
	snap, err := db.GetSnapshotByID(ctx, snapshotID)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.runClaimedSnapshot: get snapshot: %w", err)
	}
	dir, err := archive.MkdirSnapshot(archiveRoot, snap.Timestamp)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.runClaimedSnapshot: mkdir: %w", err)
	}

	runs, err := db.ListRunsBySnapshot(ctx, snapshotID)
	if err != nil {
		return Result{}, fmt.Errorf("ingest.runClaimedSnapshot: list runs: %w", err)
	}
	registry := extractorByName()
	for i := range runs {
		r := &runs[i]
		if r.Status != extractors.StatusPending && r.Status != extractors.StatusRunning {
			continue // already terminal (e.g. a prior partial run)
		}
		if r.Status == extractors.StatusPending {
			if err := db.StartRun(ctx, r.ID); err != nil {
				slog.Warn("ingest: start run", "extractor", r.Extractor, "err", err)
				continue
			}
			r.StartedAt = nowMicros()
			r.Status = extractors.StatusRunning
		}
		status, errMsg := runOne(ctx, db, registry, r, snap, dir)
		title := archive.BestTitle(dir)
		if title != "" && title != snap.Title {
			if err := db.UpdateSnapshot(ctx, snap.Timestamp, title); err != nil {
				slog.Warn("ingest: update title", "err", err)
			}
			snap.Title = title
		}
		rebuildIndex(ctx, db, dir, snap)
		if err := db.FinishRun(ctx, r.ID, status, nowMicros(), errMsg); err != nil {
			slog.Warn("ingest: finish run", "extractor", r.Extractor, "err", err)
		}
	}

	return Result{SnapshotID: snapshotID, Timestamp: snap.Timestamp, Title: snap.Title, Dir: dir}, nil
}

// runOne executes a single extractor run and records its step outputs into the
// DB. It returns the run's terminal status and (for failures) an error message;
// the caller is responsible for calling FinishRun after serializing index.json.
// A skipped extractor (ErrSkipped) records no outputs; other failures record
// the error. Failures are logged at warn but never abort the snapshot.
func runOne(ctx context.Context, db *meta.DB, registry map[string]extractors.Extractor, r *meta.ExtractorRun, snap meta.Snapshot, dir string) (string, string) {
	ex, ok := registry[r.Extractor]
	if !ok {
		slog.Warn("ingest: no extractor registered", "extractor", r.Extractor)
		return extractors.StatusFailed, "no extractor registered for " + r.Extractor
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
	return status, errMsg
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
//
// The caller must hold the snapshot lock: a run for this snapshot must be
// "running" while rebuildIndex writes. The single-running-run-per-snapshot
// invariant (see meta.ClaimNextSnapshot) is what serializes index.json writes
// across workers, so two workers never interleave writes to the same snapshot's
// index.json. Calling this outside a run's "running" window risks a torn write
// if another worker is archiving the same snapshot.
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

func nowMicros() int64 { return time.Now().UnixMicro() }
