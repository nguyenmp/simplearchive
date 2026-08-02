//go:build chromedp

// Package chromedpproxy wraps the headless-Chromium extractor so it runs via a
// SOCKS5 proxy and writes its outputs to *_proxy.* filenames.
package chromedpproxy

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedp"
)

const (
	ScreenshotFile = "screenshot_proxy.png"
	PDFFile        = "output_proxy.pdf"
	DOMFile        = "dom_chromedp_proxy.html"
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

// Run delegates to chromedp.Extractor with the proxy configured into a temp
// directory, then moves the output files to dir with *_proxy.* names so the
// original chromedp outputs are not overwritten.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	tmpDir, tmpErr := os.MkdirTemp("", "chromedp_proxy-*")
	if tmpErr != nil {
		return nil, tmpErr
	}
	defer os.RemoveAll(tmpDir)

	inner := chromedp.Extractor{
		Bin:       e.Bin,
		Timeout:   e.Timeout,
		ProxyURL:  e.ProxyURL,
		RemoteURL: e.RemoteURL,
	}
	steps, err := inner.Run(ctx, pageURL, tmpDir)

	nameMap := map[string]string{
		chromedp.ScreenshotFile: ScreenshotFile,
		chromedp.PDFFile:        PDFFile,
		chromedp.DOMFile:        DOMFile,
	}
	stepNameMap := map[string]string{
		chromedp.ScreenshotFile: "screenshot_proxy",
		chromedp.PDFFile:        "pdf_proxy",
		chromedp.DOMFile:        "chromedp_dom_proxy",
	}
	for i := range steps {
		origFile := steps[i].Filename
		steps[i].Name = stepNameMap[origFile]
		steps[i].Filename = nameMap[origFile]

		if origFile == "" {
			continue
		}
		tmpPath := filepath.Join(tmpDir, origFile)
		dstPath := filepath.Join(dir, nameMap[origFile])
		if _, statErr := os.Stat(tmpPath); statErr == nil {
			if renameErr := os.Rename(tmpPath, dstPath); renameErr != nil {
				_ = os.Link(tmpPath, dstPath)
			}
		}
	}
	return steps, err
}
