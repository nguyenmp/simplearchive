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
	// request is tried first; if it fails for any reason, the request is
	// retried via the SOCKS5 proxy.
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

	directFn := func() (string, error) { return Fetch(ctx, pageURL, dir) }
	proxyFn := func() (string, error) {
		t, transportErr := proxyutil.Transport(e.ProxyURL)
		if transportErr != nil {
			return "", fmt.Errorf("headers: proxy transport: %w", transportErr)
		}
		client := &http.Client{Timeout: 60 * time.Second, Transport: t}
		return FetchWithClient(ctx, pageURL, dir, client)
	}

	_, usedProxy, err := proxyutil.TryDirectThenProxy(directFn, proxyFn, e.ProxyURL)

	if err == nil {
		step.Status = extractors.StatusSucceeded
		step.EndTs = time.Now()
		return []extractors.Step{step}, nil
	}

	step.Status = extractors.StatusFailed
	if usedProxy {
		step.Err = fmt.Errorf("headers %q %w", pageURL, err)
	} else {
		step.Err = err
	}
	step.EndTs = time.Now()
	return []extractors.Step{step}, step.Err
}
