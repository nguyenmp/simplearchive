// Package ingest runs the archiving pipeline for a single URL. The pipeline is
// modeled as a set of independent extractor steps persisted as pending
// extractor_runs rows (Enqueue), then drained by the serve worker (RunNext),
// which runs each step, records its outputs, and rebuilds the per-snapshot
// index.json as each extractor finishes. Steps are independent: no step is
// fatal to the others, and a snapshot's state is derived from its
// extractor_runs (see meta.Snapshot), not stored on the snapshot.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nguyenmp/simplearchive/internal/archive"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedp"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedpproxy"
	"github.com/nguyenmp/simplearchive/internal/extractors/curl"
	"github.com/nguyenmp/simplearchive/internal/extractors/headers"
	"github.com/nguyenmp/simplearchive/internal/extractors/obelisk"
	"github.com/nguyenmp/simplearchive/internal/extractors/obeliskproxy"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
	"github.com/nguyenmp/simplearchive/internal/extractors/ytdlp"
	"github.com/nguyenmp/simplearchive/internal/meta"
	"github.com/nguyenmp/simplearchive/internal/proxyutil"
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
// are independent: no step is fatal to the others. The order determines
// execution order (and thus how quickly the UI shows progress). Title is
// derived incrementally after each run (see archive.BestTitle), so no single
// extractor gates title discovery.   Thus we prioritize what will give us the
// fastest success rate/title discovery first, with the slowest extractors last.
func defaultPipeline() []extractors.Extractor {
	proxy := proxyutil.EnvVar()
	chromeRemoteURL := os.Getenv("CHROME_CDP_URL")
	pipeline := []extractors.Extractor{
		// Fast (~0-2s): lightweight fetches that return quickly and cover most sites.
		wget.FaviconExtractor{},
		headers.Extractor{ProxyURL: proxy},
		curl.Extractor{ProxyURL: proxy},
		wget.HTMLExtractor{},
		// curl/wget fetches HTML only (no images), so it is consistently fast;
		// obelisk inlines resources and can be slow (8s) on image-heavy pages.
		obelisk.Extractor{},
		// Diverse (~5-30s): fails fast for non-video URLs; success is far more
		// valuable than the slower extractors below because YouTube/IG
		// typically block wget/curl/obelisk.
		ytdlp.Extractor{Cookies: os.Getenv("YT_DLP_COOKIES"), ProxyURL: proxy},
	}
	if proxy != "" {
		// (~4-15s): faster than chromedp but slower than everything else
		pipeline = append(pipeline,
			obeliskproxy.Extractor{ProxyURL: proxy},
		)
	}
	// Slow (~10-22s): headless browser; always last.
	pipeline = append(pipeline, chromedp.Extractor{RemoteURL: chromeRemoteURL})
	if proxy != "" {
		pipeline = append(pipeline,
			chromedpproxy.Extractor{ProxyURL: proxy, RemoteURL: chromeRemoteURL},
		)
	}
	return pipeline
}

// DefaultExtractorNames returns the extractor registry names in the default
// pipeline. It is exported so the web UI can populate a re-run dropdown.
func DefaultExtractorNames() []string {
	names := make([]string, 0, len(defaultPipeline()))
	for _, ex := range defaultPipeline() {
		names = append(names, ex.Name())
	}
	return names
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
	timestamp := snapshot.NewTimestamp()
	snapshotID, err := db.CreateSnapshot(ctx, url, timestamp)
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
	return snapshotID, timestamp, nil
}

// RunNext claims and archives one waiting snapshot (the worker loop's unit).
// It returns the archived Result and ran=true when it archived a snapshot, or
// a zero Result and ran=false when no snapshot was waiting. The caller loops,
// sleeping briefly between false results.
func RunNext(ctx context.Context, db *meta.DB, archiveRoot string, log *slog.Logger) (Result, bool, error) {
	if log == nil {
		log = slog.Default()
	}
	snapshotID, ok, err := db.ClaimNextSnapshot(ctx)
	if err != nil {
		return Result{}, false, fmt.Errorf("ingest.RunNext: claim: %w", err)
	}
	if !ok {
		return Result{}, false, nil
	}
	res, err := runClaimedSnapshot(ctx, db, archiveRoot, snapshotID, log)
	if err != nil {
		return Result{SnapshotID: snapshotID}, true, fmt.Errorf("ingest.RunNext: run: %w", err)
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
func runClaimedSnapshot(ctx context.Context, db *meta.DB, archiveRoot string, snapshotID int64, log *slog.Logger) (Result, error) {
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
		run := &runs[i]
		if run.Status != extractors.StatusPending && run.Status != extractors.StatusRunning {
			continue
		}
		if run.Status == extractors.StatusPending {
			if err := db.StartRun(ctx, run.ID); err != nil {
				log.Warn("ingest: start run", "extractor", run.Extractor, "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
				continue
			}
			run.StartedAt = nowMicros()
			run.Status = extractors.StatusRunning
		}
		status, errMsg := runOne(ctx, db, registry, run, snap, dir, log)
		title := archive.BestTitle(dir)
		if title != "" && title != snap.Title {
			if err := db.UpdateSnapshot(ctx, snap.Timestamp, title); err != nil {
				log.Warn("ingest: update title", "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
			}
			snap.Title = title
		}
		rebuildIndex(ctx, db, dir, snap, log)
		if err := db.FinishRun(ctx, run.ID, status, nowMicros(), errMsg); err != nil {
			log.Warn("ingest: finish run", "extractor", run.Extractor, "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
		}
	}

	return Result{SnapshotID: snapshotID, Timestamp: snap.Timestamp, Title: snap.Title, Dir: dir}, nil
}

// runOne executes a single extractor run and records its step outputs into the
// DB. It returns the run's terminal status and (for failures) an error message;
// the caller is responsible for calling FinishRun after serializing index.json.
// A skipped extractor (ErrSkipped) records no outputs; other failures record
// the error. Failures are logged at warn but never abort the snapshot.
func runOne(ctx context.Context, db *meta.DB, registry map[string]extractors.Extractor, run *meta.ExtractorRun, snap meta.Snapshot, dir string, log *slog.Logger) (string, string) {
	extractor, ok := registry[run.Extractor]
	if !ok {
		log.Warn("ingest: no extractor registered", "extractor", run.Extractor, "url", snap.URL, "timestamp", snap.Timestamp)
		return extractors.StatusFailed, "no extractor registered for " + run.Extractor
	}
	steps, runErr := extractor.Run(ctx, snap.URL, dir)
	if runErr != nil && !errors.Is(runErr, extractors.ErrSkipped) {
		log.Warn("ingest: extractor failed", "extractor", extractor.Name(), "url", snap.URL, "timestamp", snap.Timestamp, "err", runErr)
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

	if err := db.DeleteStepOutputs(ctx, run.ID); err != nil {
		log.Warn("ingest: clear step outputs", "extractor", extractor.Name(), "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
	}
	for _, step := range steps {
		out := meta.StepOutput{
			RunID:    run.ID,
			Name:     step.Name,
			Filename: step.Filename,
			Cmd:      step.Cmd,
			Status:   step.Status,
			StartTs:  step.StartTs.UnixMicro(),
			EndTs:    step.EndTs.UnixMicro(),
		}
		if step.Err != nil {
			out.Error = step.Err.Error()
		}
		if _, err := db.InsertStepOutput(ctx, run.ID, out); err != nil {
			log.Warn("ingest: record step output", "extractor", extractor.Name(), "step", step.Name, "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
		}
	}
	return status, errMsg
}

// aggregateRunStatus derives an extractor run's status from its steps: failed
// if any step failed, succeeded if any succeeded, else skipped.
func aggregateRunStatus(steps []extractors.Step) string {
	var anySucceeded, anyFailed bool
	for _, step := range steps {
		switch step.Status {
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
func rebuildIndex(ctx context.Context, db *meta.DB, dir string, snap meta.Snapshot, log *slog.Logger) {
	runs, err := db.ListRunsBySnapshot(ctx, snap.ID)
	if err != nil {
		log.Warn("ingest: list runs for index", "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
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
		log.Warn("ingest: rebuild index.json", "url", snap.URL, "timestamp", snap.Timestamp, "err", err)
	}
}

// runsToSteps flattens a snapshot's terminal runs' outputs into the
// extractors.Step list archive.WriteIndex expects. Skipped/failed runs with no
// outputs contribute nothing; the per-output status is what index.json records.
func runsToSteps(runs []meta.ExtractorRun) []extractors.Step {
	out := make([]extractors.Step, 0)
	for _, run := range runs {
		for _, stepOutput := range run.Outputs {
			out = append(out, extractors.Step{
				Name:     stepOutput.Name,
				Filename: stepOutput.Filename,
				Cmd:      stepOutput.Cmd,
				Status:   stepOutput.Status,
			})
		}
	}
	return out
}

func nowMicros() int64 { return time.Now().UnixMicro() }
