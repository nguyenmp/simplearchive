// Package obeliskproxy wraps the go-shiori/obelisk library to archive a URL
// via a SOCKS5 proxy into a single self-contained HTML file.
package obeliskproxy

import (
	"context"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/obelisk"
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
	return obelisk.RunObelisk(ctx, pageURL, dir, e.ProxyURL, "singlefile_proxy", OutputFile)
}
