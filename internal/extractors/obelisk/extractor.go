// Package obelisk wraps the go-shiori/obelisk library to archive a URL into a
// single self-contained HTML file.
package obelisk

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	ob "github.com/go-shiori/obelisk"
)

// OutputFile is the filename the single-file HTML is written to.
const OutputFile = "singlefile.html"

// Extractor archives a URL into a single self-contained HTML file via obelisk.
type Extractor struct{}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "obelisk" }

// Run archives url into dir/singlefile.html and reports a single "singlefile"
// step. It runs in-process, so the recorded cmd is empty.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	arc := ob.Archiver{
		RequestTimeout: 60 * time.Second,
	}
	arc.Validate()
	content, _, err := arc.Archive(ctx, ob.Request{URL: pageURL})
	end := time.Now()

	step := extractors.Step{
		Name:     "singlefile",
		Filename: OutputFile,
		StartTs:  start,
		EndTs:    end,
	}
	if err != nil {
		step.Status = extractors.StatusFailed
		step.Err = err
		return []extractors.Step{step}, err
	}
	if werr := os.WriteFile(filepath.Join(dir, OutputFile), content, 0o644); werr != nil {
		step.Status = extractors.StatusFailed
		step.Err = werr
		return []extractors.Step{step}, werr
	}
	step.Status = extractors.StatusSucceeded
	return []extractors.Step{step}, nil
}
