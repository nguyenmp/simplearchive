//go:build !chromedp

// Package chromedpproxy is a no-op when built without the "chromedp" build tag.
package chromedpproxy

import (
	"context"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// Extractor is a no-op when the chromedp build tag is not enabled.
type Extractor struct {
	Bin       string
	Timeout   time.Duration
	ProxyURL  string
	RemoteURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "chromedp_proxy" }

// Run always returns ErrSkipped when the chromedp build tag is not enabled.
func (Extractor) Run(context.Context, string, string) ([]extractors.Step, error) {
	return nil, extractors.ErrSkipped
}
