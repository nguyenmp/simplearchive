package wget

import (
	"context"
	"net/url"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

// DOMExtractor archives a URL's page body via wget into output.html.
type DOMExtractor struct{}

// Name returns the extractor registry identifier.
func (DOMExtractor) Name() string { return "wget" }

// Run fetches url into dir/output.html and reports a single "dom" step.
func (e DOMExtractor) Run(ctx context.Context, pageURL, dir string) ([]extractors.Step, error) {
	start := time.Now()
	path, err := Fetch(ctx, pageURL, dir)
	end := time.Now()
	step := extractors.Step{
		Name:     "dom",
		Filename: OutputFile,
		Cmd:      []string{"wget", "--no-verbose", "--output-document=" + filepath.Join(dir, OutputFile), pageURL},
		StartTs:  start,
		EndTs:    end,
	}
	if err != nil {
		step.Status = extractors.StatusFailed
		step.Err = err
		return []extractors.Step{step}, err
	}
	step.Status = extractors.StatusSucceeded
	_ = path
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
	path, err := FetchFavicon(ctx, pageURL, dir)
	end := time.Now()
	step := extractors.Step{
		Name:     "favicon",
		Filename: FaviconFile,
		Cmd:      faviconCmd(pageURL, dir),
		StartTs:  start,
		EndTs:    end,
	}
	if err != nil {
		step.Status = extractors.StatusFailed
		step.Err = err
		return []extractors.Step{step}, err
	}
	step.Status = extractors.StatusSucceeded
	_ = path
	return []extractors.Step{step}, nil
}

// faviconCmd builds the shell argv recorded for the favicon step, mirroring
// archive.commandFor. It returns an empty list when the page URL cannot be
// parsed (no host to look up).
func faviconCmd(pageURL, dir string) []string {
	u, err := url.Parse(pageURL)
	if err != nil || u.Host == "" {
		return nil
	}
	return []string{"wget", "--no-verbose", "--output-document=" + filepath.Join(dir, FaviconFile), faviconService + u.Hostname()}
}
