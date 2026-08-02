//go:build chromedp

// Package chromedpproxy wraps the headless-Chromium extractor so it runs via a
// SOCKS5 proxy and writes its outputs to *_proxy.* filenames.
package chromedpproxy

import (
	"context"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedp"
)

// Extractor is the proxy variant of chromedp.Extractor.
type Extractor struct {
	Bin       string
	Timeout   time.Duration
	ProxyURL  string
	RemoteURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "chromedp_proxy" }

// Run delegates to chromedp.Extractor with the proxy configured and
// FileSuffix="_proxy" so outputs are written to *_proxy.* filenames in dir,
// leaving the original chromedp outputs untouched.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	inner := chromedp.Extractor{
		Bin:        e.Bin,
		Timeout:    e.Timeout,
		ProxyURL:   e.ProxyURL,
		RemoteURL:  e.RemoteURL,
		FileSuffix: "_proxy",
	}
	return inner.Run(ctx, pageURL, dir)
}
