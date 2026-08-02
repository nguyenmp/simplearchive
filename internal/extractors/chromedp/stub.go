//go:build !chromedp

// Package chromedp provides a headless-Chromium extractor (screenshot, PDF,
// JS-rendered DOM). The real implementation is compiled in only with the
// "chromedp" build tag (it pulls in the chromedp library and a Chromium
// binary). Without the tag this stub is used so the pipeline can include the
// extractor unconditionally and it simply skips.
package chromedp

import (
	"context"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// Extractor is a no-op when built without the "chromedp" build tag.
type Extractor struct {
	Bin     string
	Timeout time.Duration
	// ProxyURL is ignored in the stub build.
	ProxyURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "chromedp" }

// Run always returns ErrSkipped when the chromedp build tag is not enabled.
func (Extractor) Run(context.Context, string, string) ([]extractors.Step, error) {
	return nil, extractors.ErrSkipped
}
