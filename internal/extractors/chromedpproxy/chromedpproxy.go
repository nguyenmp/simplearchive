//go:build chromedp

// Package chromedpproxy wraps the headless-Chromium extractor so it runs via a
// SOCKS5 proxy and writes its outputs to *_proxy.* filenames.
package chromedpproxy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/extractors/chromedp"
)

// Extractor is the proxy variant of chromedp.Extractor.
type Extractor struct {
	Bin      string
	Timeout  time.Duration
	ProxyURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "chromedp_proxy" }

// Run delegates to chromedp.Extractor with the proxy configured, then renames
// the output files to their *_proxy.* equivalents and updates step names.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	inner := chromedp.Extractor{
		Bin:      e.Bin,
		Timeout:  e.Timeout,
		ProxyURL: e.ProxyURL,
	}
	steps, err := inner.Run(ctx, pageURL, dir)
	for i := range steps {
		oldName := steps[i].Name
		steps[i].Name = renameStep(oldName)

		oldFile := steps[i].Filename
		newFile := renameFile(oldFile)
		steps[i].Filename = newFile

		// Rename on disk if the old file was written successfully.
		if oldFile != "" && oldFile != newFile {
			oldPath := filepath.Join(dir, oldFile)
			newPath := filepath.Join(dir, newFile)
			if _, statErr := os.Stat(oldPath); statErr == nil {
				_ = os.Rename(oldPath, newPath)
			}
		}
	}
	return steps, err
}

func renameStep(name string) string {
	return name + "_proxy"
}

func renameFile(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name + "_proxy"
	}
	base := strings.TrimSuffix(name, ext)
	return base + "_proxy" + ext
}
