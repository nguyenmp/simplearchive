//go:build chromedp

// Package chromedp provides a headless-Chromium extractor (screenshot, PDF,
// JS-rendered DOM). This file is compiled in only with the "chromedp" build
// tag. At runtime it still skips (ErrSkipped) when no Chromium binary is found,
// so the same binary runs with or without the tag.
package chromedp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/page"
	cdp "github.com/chromedp/chromedp"
	"github.com/nguyenmp/simplearchive/internal/extractors"
)

const (
	ScreenshotFile = "screenshot.png"
	PDFFile        = "output.pdf"
	DOMFile        = "dom_chromedp.html"
)

// Extractor archives a URL via headless Chromium into screenshot.png,
// output.pdf, and dom_chromedp.html.
type Extractor struct {
	// Bin is the Chromium binary path; defaults to "chromium" when empty.
	Bin string
	// Timeout caps a single archive. Defaults to 60s when zero.
	Timeout time.Duration
	// ProxyURL is an optional socks5:// URL passed to Chromium via
	// --proxy-server. When empty no proxy is used.
	ProxyURL string
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
	return 60 * time.Second
}

// Run archives url into dir. It returns ErrSkipped when no Chromium binary is
// available. On failure every step is reported with StatusFailed and the
// cause; on success each output is written and marked individually.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	bin := e.bin()
	if _, err := exec.LookPath(bin); err != nil {
		return nil, extractors.ErrSkipped
	}

	start := time.Now()
	opts := append(cdp.DefaultExecAllocatorOptions[:],
		cdp.ExecPath(bin),
		cdp.NoSandbox, // containers typically run as root; sandbox needs a user namespace
	)
	if e.ProxyURL != "" {
		opts = append(opts, cdp.ProxyServer(e.ProxyURL))
	}
	allocCtx, cancel := cdp.NewExecAllocator(ctx, opts...)
	defer cancel()
	taskCtx, cancel := cdp.NewContext(allocCtx)
	defer cancel()
	taskCtx, cancel = context.WithTimeout(taskCtx, e.timeout())
	defer cancel()

	var screenshot []byte
	var pdf []byte
	var dom string
	runErr := cdp.Run(taskCtx,
		cdp.Navigate(pageURL),
		cdp.FullScreenshot(&screenshot, 100),
		cdp.ActionFunc(func(ctx context.Context) error {
			var perr error
			pdf, _, perr = page.PrintToPDF().WithPrintBackground(true).Do(ctx)
			return perr
		}),
		cdp.OuterHTML("html", &dom),
	)
	end := time.Now()

	cmd := []string{bin, "--headless", "--no-sandbox", pageURL}
	steps := []extractors.Step{
		{Name: "screenshot", Filename: ScreenshotFile, Cmd: cmd, StartTs: start, EndTs: end},
		{Name: "pdf", Filename: PDFFile, Cmd: cmd, StartTs: start, EndTs: end},
		{Name: "chromedp_dom", Filename: DOMFile, Cmd: cmd, StartTs: start, EndTs: end},
	}
	if runErr != nil {
		for i := range steps {
			steps[i].Status = extractors.StatusFailed
			steps[i].Err = runErr
		}
		return steps, runErr
	}
	writeStep := func(i int, name string, data []byte) {
		if werr := os.WriteFile(filepath.Join(dir, name), data, 0o644); werr != nil {
			steps[i].Status = extractors.StatusFailed
			steps[i].Err = werr
			return
		}
		steps[i].Status = extractors.StatusSucceeded
	}
	writeStep(0, ScreenshotFile, screenshot)
	writeStep(1, PDFFile, pdf)
	writeStep(2, DOMFile, []byte(dom))
	return steps, nil
}
