//go:build chromedp

package chromedp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/page"
	chromedplib "github.com/chromedp/chromedp"
	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// Run archives url into dir. In local mode it returns ErrSkipped when no
// Chromium binary is available. On failure every step is reported with
// StatusFailed and the cause; on success each output is written and marked
// individually.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	log := slog.With("extractor", e.Name()+e.FileSuffix, "url", pageURL)
	allocCtx, cancel, cmd, err := e.allocator(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	defer cancel()
	taskCtx, cancel := chromedplib.NewContext(allocCtx)
	defer cancel()
	timeout := e.timeout()
	taskCtx, cancel = context.WithTimeout(taskCtx, timeout)
	defer cancel()

	if deadline, ok := ctx.Deadline(); ok {
		log.Info("chromedp: starting", "extractor_timeout", timeout, "parent_deadline", time.Until(deadline).Round(time.Second))
	} else {
		log.Info("chromedp: starting", "extractor_timeout", timeout, "parent_deadline", "none")
	}

	start := time.Now()
	screenshot, pdf, dom, runErr := e.captureOutputs(taskCtx, pageURL)
	elapsed := time.Since(start)

	if runErr != nil {
		reason := classifyFailure(ctx, taskCtx)
		log.Warn("chromedp: failed", "elapsed", elapsed.Round(time.Second), "reason", reason, "err", runErr)
	} else {
		log.Info("chromedp: succeeded", "elapsed", elapsed.Round(time.Second))
	}

	end := start.Add(elapsed)
	steps := []extractors.Step{
		extractors.NewOutput("screenshot"+e.FileSuffix, e.screenshotFile(), start, end, nil),
		extractors.NewOutput("pdf"+e.FileSuffix, e.pdfFile(), start, end, nil),
		extractors.NewOutput("chromedp_dom"+e.FileSuffix, e.domFile(), start, end, nil),
	}
	for i := range steps {
		steps[i].Cmd = cmd
	}
	if runErr != nil {
		for i := range steps {
			steps[i].Status = extractors.StatusFailed
			steps[i].Err = runErr
		}
		return steps, runErr
	}
	if writeErr := writeStepFiles(dir, steps, screenshot, pdf, dom); writeErr != nil {
		return steps, writeErr
	}
	return steps, nil
}

func (e Extractor) captureOutputs(ctx context.Context, pageURL string) (screenshot []byte, pdf []byte, dom string, err error) {
	err = chromedplib.Run(ctx,
		chromedplib.Navigate(pageURL),
		chromedplib.FullScreenshot(&screenshot, 100),
		chromedplib.ActionFunc(func(ctx context.Context) error {
			var perr error
			pdf, _, perr = page.PrintToPDF().WithPrintBackground(true).Do(ctx)
			return perr
		}),
		chromedplib.OuterHTML("html", &dom),
	)
	return
}

func classifyFailure(ctx context.Context, taskCtx context.Context) string {
	if context.Cause(ctx) != nil {
		return fmt.Sprintf("parent canceled: %v", context.Cause(ctx))
	}
	if ctx.Err() == context.Canceled {
		return "parent canceled"
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "parent deadline exceeded"
	}
	if taskCtx.Err() == context.DeadlineExceeded {
		return "extractor timeout"
	}
	return "unknown"
}

func writeStepFiles(dir string, steps []extractors.Step, screenshot []byte, pdf []byte, dom string) error {
	type write struct {
		idx  int
		data []byte
	}
	writes := []write{{0, screenshot}, {1, pdf}, {2, []byte(dom)}}
	var firstErr error
	for _, w := range writes {
		if err := os.WriteFile(filepath.Join(dir, steps[w.idx].Filename), w.data, extractors.DefaultFilePerm); err != nil {
			steps[w.idx].Status = extractors.StatusFailed
			steps[w.idx].Err = err
			if firstErr == nil {
				firstErr = err
			}
		} else {
			steps[w.idx].Status = extractors.StatusSucceeded
		}
	}
	return firstErr
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
		allocCtx, cancel := chromedplib.NewRemoteAllocator(ctx, wsURL, chromedplib.NoModifyURL)
		return allocCtx, cancel, []string{"chromedp", wsURL, pageURL}, nil
	}

	bin := e.bin()
	if _, err := exec.LookPath(bin); err != nil {
		slog.Warn("chromedp: skipping, browser binary not found", "bin", bin, "err", err)
		return nil, nil, nil, extractors.ErrSkipped
	}
	opts := append(chromedplib.DefaultExecAllocatorOptions[:],
		chromedplib.ExecPath(bin),
		chromedplib.NoSandbox,
	)
	if e.ProxyURL != "" {
		opts = append(opts, chromedplib.ProxyServer(e.ProxyURL))
	}
	allocCtx, cancel := chromedplib.NewExecAllocator(ctx, opts...)
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
		queryParams := u.Query()
		queryParams.Set("--proxy-server", proxyURL)
		u.RawQuery = queryParams.Encode()
	}
	return u.String(), nil
}
