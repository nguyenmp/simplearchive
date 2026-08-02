package curl

import (
	"context"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/proxyutil"
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

	_, usedProxy, err := proxyutil.TryDirectThenProxy(
		func() (string, error) { return Fetch(ctx, pageURL, dir, "") },
		func() (string, error) { return Fetch(ctx, pageURL, dir, e.ProxyURL) },
		e.ProxyURL,
	)
	end := time.Now()

	proxyURL := ""
	if usedProxy {
		proxyURL = e.ProxyURL
	}
	step := extractors.NewOutput("curl", OutputFile, start, end, err)
	step.Cmd = buildCmd(pageURL, dir, proxyURL)
	if err != nil {
		return []extractors.Step{step}, err
	}
	return []extractors.Step{step}, nil
}

func buildCmd(pageURL, dir, proxyURL string) []string {
	return append([]string{"curl"}, fetchArgv(pageURL, filepath.Join(dir, OutputFile), proxyURL)...)
}
