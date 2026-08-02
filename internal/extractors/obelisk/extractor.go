// Package obelisk wraps the go-shiori/obelisk library to archive a URL into a
// single self-contained HTML file.
package obelisk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	obelisklib "github.com/go-shiori/obelisk"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/proxyutil"
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
	return RunObelisk(ctx, pageURL, dir, "", "singlefile", OutputFile)
}

// RunObelisk is the shared implementation for obelisk-based extractors. It
// archives pageURL into dir/outputFile using an optional SOCKS5 proxyURL and
// reports a single step named stepName. Both the obelisk and obeliskproxy
// extractors delegate to this function.
func RunObelisk(ctx context.Context, pageURL, dir, proxyURL, stepName, outputFile string) ([]extractors.Step, error) {
	start := time.Now()
	archiver := obelisklib.Archiver{
		RequestTimeout: extractors.DefaultTimeout,
	}
	if transport, err := proxyutil.Transport(proxyURL); err != nil {
		return nil, fmt.Errorf("obelisk: %w", err)
	} else if transport != nil {
		archiver.Transport = transport
	}
	archiver.Validate()
	content, _, err := archiver.Archive(ctx, obelisklib.Request{URL: pageURL})
	end := time.Now()

	if err != nil {
		return []extractors.Step{extractors.NewOutput(stepName, outputFile, start, end, err)}, err
	}
	if werr := os.WriteFile(filepath.Join(dir, outputFile), content, extractors.DefaultFilePerm); werr != nil {
		return []extractors.Step{extractors.NewOutput(stepName, outputFile, start, end, werr)}, werr
	}
	return []extractors.Step{extractors.NewOutput(stepName, outputFile, start, end, nil)}, nil
}
