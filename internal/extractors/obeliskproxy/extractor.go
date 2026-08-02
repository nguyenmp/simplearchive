// Package obeliskproxy wraps the go-shiori/obelisk library to archive a URL
// via a SOCKS5 proxy into a single self-contained HTML file.
package obeliskproxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ob "github.com/go-shiori/obelisk"
	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/proxyutil"
)

// OutputFile is the filename the single-file HTML is written to.
const OutputFile = "singlefile_proxy.html"

// Extractor archives a URL into a single self-contained HTML file via obelisk
// through a SOCKS5 proxy.
type Extractor struct {
	ProxyURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "obelisk_proxy" }

// Run archives url into dir/singlefile_proxy.html and reports a single
// "singlefile_proxy" step.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	arc := ob.Archiver{
		RequestTimeout: 60 * time.Second,
	}
	if t, err := proxyutil.Transport(e.ProxyURL); err != nil {
		return nil, fmt.Errorf("obeliskproxy: %w", err)
	} else if t != nil {
		arc.Transport = t
	}
	arc.Validate()
	content, _, err := arc.Archive(ctx, ob.Request{URL: pageURL})
	end := time.Now()

	step := extractors.Step{
		Name:     "singlefile_proxy",
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
