// Package chromedp provides a headless-Chromium extractor (screenshot, PDF,
// JS-rendered DOM). The real implementation is compiled in only with the
// "chromedp" build tag (it pulls in the chromedp library and a Chromium
// binary). Without the tag a stub is used so the pipeline can include the
// extractor unconditionally and it simply skips.
package chromedp

import (
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

const (
	ScreenshotFile = "screenshot.png"
	PDFFile        = "output.pdf"
	DOMFile        = "dom_chromedp.html"
)

// Extractor archives a URL via headless Chromium into screenshot.png,
// output.pdf, and dom_chromedp.html (or with a FileSuffix, e.g.
// screenshot_proxy.png).
type Extractor struct {
	// Bin is the Chromium binary path; defaults to "chromium" when empty.
	// Ignored when RemoteURL is set.
	Bin string
	// Timeout caps a single archive. Defaults to 60s when zero.
	Timeout time.Duration
	// ProxyURL is an optional socks5:// URL. For a local browser it is passed
	// to Chromium via --proxy-server; for a remote browser it is appended to
	// the websocket URL's query string, which CDP proxies like
	// sockpuppetbrowser turn into a Chrome launch flag. When empty no proxy
	// is used.
	ProxyURL string
	// RemoteURL is an optional browser-level CDP websocket URL (e.g.
	// ws://sockpuppetbrowser:3000). When set, no local browser is launched:
	// each Run opens one connection and the server maps it to a fresh Chrome.
	RemoteURL string
	// FileSuffix is appended before the file extension (e.g. "_proxy" produces
	// screenshot_proxy.png). When empty the default filenames are used.
	FileSuffix string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "chromedp" }

func (e Extractor) bin() string {
	if e.Bin != "" {
		return e.Bin
	}
	return "chromium"
}

func (e Extractor) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return extractors.DefaultTimeout
}

func (e Extractor) screenshotFile() string {
	if e.FileSuffix != "" {
		return "screenshot" + e.FileSuffix + ".png"
	}
	return ScreenshotFile
}

func (e Extractor) pdfFile() string {
	if e.FileSuffix != "" {
		return "output" + e.FileSuffix + ".pdf"
	}
	return PDFFile
}

func (e Extractor) domFile() string {
	if e.FileSuffix != "" {
		return "dom_chromedp" + e.FileSuffix + ".html"
	}
	return DOMFile
}
