package wget

import (
	"context"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// HTMLExtractor archives a URL's page body via wget into output.html.
type HTMLExtractor struct{}

// Name returns the extractor registry identifier.
func (HTMLExtractor) Name() string { return "wget" }

// Run fetches url into dir/output.html and reports a single "dom" step.
func (e HTMLExtractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	_, err := Fetch(ctx, pageURL, dir)
	end := time.Now()
	step := extractors.NewOutput("dom", OutputFile, start, end, err)
	step.Cmd = wgetArgv(filepath.Join(dir, OutputFile), pageURL)
	if err != nil {
		return []extractors.Step{step}, err
	}
	return []extractors.Step{step}, nil
}

// FaviconExtractor fetches the site favicon via Google's favicon service.
type FaviconExtractor struct{}

// Name returns the extractor registry identifier.
func (FaviconExtractor) Name() string { return "wget-favicon" }

// Run fetches the favicon for pageURL into dir/favicon.ico and reports a
// single "favicon" step. A failure (invalid URL or fetch error) is reported
// with StatusFailed; ingest treats favicon as best-effort.
func (e FaviconExtractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	_, err := FetchFavicon(ctx, pageURL, dir)
	end := time.Now()
	step := extractors.NewOutput("favicon", FaviconFile, start, end, err)
	if argv, argvErr := faviconArgv(pageURL, dir); argvErr == nil {
		step.Cmd = argv
	}
	if err != nil {
		return []extractors.Step{step}, err
	}
	return []extractors.Step{step}, nil
}
