package headers

import (
	"context"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// Extractor fetches HTTP response headers for a URL via net/http.
type Extractor struct{}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "headers" }

// Run fetches headers for pageURL into dir/headers.json and reports a single
// "headers" step. It runs in-process, so the recorded cmd is empty (matching
// archive.commandFor's handling of the headers extractor).
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	path, err := Fetch(ctx, pageURL, dir)
	end := time.Now()
	step := extractors.Step{
		Name:     "headers",
		Filename: OutputFile,
		Cmd:      nil,
		StartTs:  start,
		EndTs:    end,
	}
	if err != nil {
		step.Status = extractors.StatusFailed
		step.Err = err
		return []extractors.Step{step}, err
	}
	step.Status = extractors.StatusSucceeded
	_ = path
	return []extractors.Step{step}, nil
}
