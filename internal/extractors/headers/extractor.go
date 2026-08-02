package headers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/proxyutil"
)

// Extractor fetches HTTP response headers for a URL via net/http.
type Extractor struct {
	// ProxyURL is an optional socks5:// URL. When non-empty, a direct HEAD
	// request is tried first; if the response status code is in the 4xx range
	// the request is retried via the SOCKS5 proxy.
	ProxyURL string
}

// Name returns the extractor registry identifier.
func (Extractor) Name() string { return "headers" }

// Run fetches headers for pageURL into dir/headers.json and reports a single
// "headers" step. It tries a direct request first. If ProxyURL is set and the
// direct response has a 4xx status code, it retries via the proxy and the
// proxy result (success or failure) becomes the recorded step.
func (e Extractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	step := extractors.Step{
		Name:     "headers",
		Filename: OutputFile,
		Cmd:      nil,
		StartTs:  start,
	}

	// Direct attempt.
	path, directErr := Fetch(ctx, pageURL, dir)
	if directErr == nil {
		step.Status = extractors.StatusSucceeded
		step.EndTs = time.Now()
		_ = path
		return []extractors.Step{step}, nil
	}

	// If proxy is configured, try again with proxy.
	if e.ProxyURL != "" {
		t, err := proxyutil.Transport(e.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("headers: proxy transport: %w", err)
		}
		client := &http.Client{
			Timeout:   60 * time.Second,
			Transport: t,
		}
		path, proxyErr := FetchWithClient(ctx, pageURL, dir, client)
		if proxyErr == nil {
			step.Status = extractors.StatusSucceeded
			step.EndTs = time.Now()
			_ = path
			return []extractors.Step{step}, nil
		}
		step.Status = extractors.StatusFailed
		step.Err = fmt.Errorf("direct: %w; proxy: %w", directErr, proxyErr)
		step.EndTs = time.Now()
		return []extractors.Step{step}, step.Err
	}

	step.Status = extractors.StatusFailed
	step.Err = directErr
	step.EndTs = time.Now()
	return []extractors.Step{step}, directErr
}
