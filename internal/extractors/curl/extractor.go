package curl

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// Extractor fetches a URL via curl into curl.html. When ProxyURL is set it
// tries a direct request first and falls back to the proxy on failure.
type Extractor struct {
	ProxyURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "curl" }

// Run fetches url into dir/curl.html and reports a single "curl" step.
// It always attempts a direct fetch first. If that fails and ProxyURL is
// non-empty, it retries via the SOCKS5 proxy. The step reflects the final
// attempt (direct when it succeeds, proxy when the proxy fallback succeeds).
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	step := extractors.Step{
		Name:     "curl",
		Filename: OutputFile,
		StartTs:  start,
	}

	// Direct attempt.
	path, directErr := Fetch(ctx, pageURL, dir, "")
	if directErr == nil {
		step.Status = extractors.StatusSucceeded
		step.Cmd = buildCmd(pageURL, dir, "")
		step.EndTs = time.Now()
		_ = path
		return []extractors.Step{step}, nil
	}

	// Proxy fallback.
	if e.ProxyURL != "" {
		path, proxyErr := Fetch(ctx, pageURL, dir, e.ProxyURL)
		if proxyErr == nil {
			step.Status = extractors.StatusSucceeded
			step.Cmd = buildCmd(pageURL, dir, e.ProxyURL)
			step.EndTs = time.Now()
			_ = path
			return []extractors.Step{step}, nil
		}
		step.Status = extractors.StatusFailed
		step.Err = fmt.Errorf("direct: %w; proxy: %w", directErr, proxyErr)
		step.Cmd = buildCmd(pageURL, dir, e.ProxyURL)
		step.EndTs = time.Now()
		return []extractors.Step{step}, step.Err
	}

	// No proxy configured.
	step.Status = extractors.StatusFailed
	step.Err = directErr
	step.Cmd = buildCmd(pageURL, dir, "")
	step.EndTs = time.Now()
	return []extractors.Step{step}, directErr
}

func buildCmd(pageURL, dir, proxyURL string) []string {
	return append([]string{"curl"}, fetchArgv(pageURL, filepath.Join(dir, OutputFile), proxyURL)...)
}
