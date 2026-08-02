//go:build chromedp

// Package chromedp provides a headless-Chromium extractor (screenshot, PDF,
// JS-rendered DOM). This file is compiled in only with the "chromedp" build
// tag. The extractor drives either a local Chromium binary (the default) or a
// remote Chrome over a CDP websocket URL (see Extractor.RemoteURL). In local
// mode it still skips (ErrSkipped) when no Chromium binary is found, so the
// same binary runs with or without a local browser.
package chromedp

import (
	"context"
	"fmt"
	"net/url"
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
	return 60 * time.Second
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

// Run archives url into dir. In local mode it returns ErrSkipped when no
// Chromium binary is available. On failure every step is reported with
// StatusFailed and the cause; on success each output is written and marked
// individually.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	allocCtx, cancel, cmd, err := e.allocator(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	defer cancel()
	taskCtx, cancel := cdp.NewContext(allocCtx)
	defer cancel()
	taskCtx, cancel = context.WithTimeout(taskCtx, e.timeout())
	defer cancel()

	start := time.Now()
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

	steps := []extractors.Step{
		{Name: "screenshot" + e.FileSuffix, Filename: e.screenshotFile(), Cmd: cmd, StartTs: start, EndTs: end},
		{Name: "pdf" + e.FileSuffix, Filename: e.pdfFile(), Cmd: cmd, StartTs: start, EndTs: end},
		{Name: "chromedp_dom" + e.FileSuffix, Filename: e.domFile(), Cmd: cmd, StartTs: start, EndTs: end},
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
	writeStep(0, e.screenshotFile(), screenshot)
	writeStep(1, e.pdfFile(), pdf)
	writeStep(2, e.domFile(), []byte(dom))
	return steps, nil
}

// allocator builds the chromedp allocator context for the configured browser
// backend plus the argv recorded in each step's Cmd field. With RemoteURL it
// connects to a remote Chrome over CDP; otherwise it launches a local binary
// (ErrSkipped when none is on PATH).
func (e Extractor) allocator(ctx context.Context, pageURL string) (context.Context, context.CancelFunc, []string, error) {
	if e.RemoteURL != "" {
		wsURL, err := remoteWSURL(e.RemoteURL, e.ProxyURL)
		if err != nil {
			return nil, nil, nil, err
		}
		// NoModifyURL: chromedp's default URL resolution (GET /json/version on
		// the same port) does not work against CDP proxies like
		// sockpuppetbrowser, which speak raw websocket on the given URL.
		allocCtx, cancel := cdp.NewRemoteAllocator(ctx, wsURL, cdp.NoModifyURL)
		return allocCtx, cancel, []string{"chromedp", wsURL, pageURL}, nil
	}

	bin := e.bin()
	if _, err := exec.LookPath(bin); err != nil {
		return nil, nil, nil, extractors.ErrSkipped
	}
	opts := append(cdp.DefaultExecAllocatorOptions[:],
		cdp.ExecPath(bin),
		cdp.NoSandbox, // containers typically run as root; sandbox needs a user namespace
	)
	if e.ProxyURL != "" {
		opts = append(opts, cdp.ProxyServer(e.ProxyURL))
	}
	allocCtx, cancel := cdp.NewExecAllocator(ctx, opts...)
	return allocCtx, cancel, []string{bin, "--headless", "--no-sandbox", pageURL}, nil
}

// remoteWSURL builds the websocket URL to dial: base plus, when proxyURL is
// set, a --proxy-server query param. CDP proxies like sockpuppetbrowser map
// "--flag" query params onto the launched Chrome's argv, which is how a
// per-connection proxy is requested for a browser we don't launch ourselves.
// The base URL's own query params are preserved.
func remoteWSURL(base, proxyURL string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("chromedp: invalid remote URL %q: %w", base, err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", fmt.Errorf("chromedp: remote URL %q must use ws:// or wss:// scheme", base)
	}
	if proxyURL != "" {
		q := u.Query()
		q.Set("--proxy-server", proxyURL)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}
