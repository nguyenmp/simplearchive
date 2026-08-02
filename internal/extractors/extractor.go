// Package extractors defines the common interface implemented by each archive
// extractor (wget, headers, obelisk, yt-dlp, chromedp, ...). An extractor runs
// against a single URL inside a snapshot directory and emits one or more Steps
// describing the outputs it wrote.
package extractors

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

const DefaultFilePerm fs.FileMode = 0o644

// Status of an extractor step. These match the "status" field ArchiveBox records
// in each ArchiveResult of a snapshot's index.json history. The pending/running
// states are used by the queue (extractor_runs) before a step reaches a
// terminal state; they are never written to index.json.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusSkipped   = "skipped"
)

const DefaultTimeout = 60 * time.Second

// ErrSkipped is returned by an extractor that cannot run in the current build
// or environment (for example chromedp when no browser binary is available).
// Callers treat it as a best-effort skip rather than a failure.
var ErrSkipped = errors.New("extractor: skipped")

// NewOutput constructs a Step with the given name, filename, and time bracket,
// setting Status to StatusSucceeded when err is nil and StatusFailed with Err
// set when err is non-nil. Callers may override fields (e.g. Cmd) after
// construction.
func NewOutput(name, filename string, start, end time.Time, err error) Step {
	s := Step{Name: name, Filename: filename, StartTs: start, EndTs: end}
	if err != nil {
		s.Status = StatusFailed
		s.Err = err
	} else {
		s.Status = StatusSucceeded
	}
	return s
}

// Step is the result of a single named output produced by an extractor. Each
// Step maps to one entry in a snapshot's index.json "history"/"latest" maps and
// to one row in the extractor_runs table.
type Step struct {
	// Name is the ArchiveBox extractor/plugin key (e.g. "dom", "favicon",
	// "headers", "screenshot") and the extractor_runs.extractor value.
	Name string
	// Filename is the output file written to the snapshot directory (e.g.
	// "output.html"). For non-file outputs (such as a parsed title) it may be
	// empty or carry the value itself.
	Filename string
	// Cmd is the shell argv recorded in the index.json history entry's "cmd"
	// field for debuggability and ArchiveBox reimport. Empty for in-process
	// extractors.
	Cmd []string
	// Status is one of the Status* constants.
	Status string
	// Err is the failure cause when Status == StatusFailed, or nil otherwise.
	Err error
	// StartTs and EndTs bracket the step's execution.
	StartTs time.Time
	EndTs   time.Time
}

// Extractor archives a single URL into a snapshot directory. Run returns the
// Steps it produced; a non-nil error means the extractor could not run at all
// (see ErrSkipped). Per-output success or failure is reported in each Step's
// Status, not via the returned error.
type Extractor interface {
	// Name is the extractor's registry/identifier (e.g. "wget", "chromedp"),
	// distinct from the per-output Step.Name. It is used for logging and, later,
	// for config-driven extractor selection.
	Name() string
	// Run archives url into dir, returning the outputs it wrote.
	Run(ctx context.Context, url, dir string) ([]Step, error)
}
