// Package chromedpproxy wraps the headless-Chromium extractor so it runs via a
// SOCKS5 proxy and writes its outputs to *_proxy.* filenames.
package chromedpproxy

import (
	"time"
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
