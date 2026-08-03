// Package wget wraps the wget command-line tool to fetch a URL into a snapshot
// directory.
package wget

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/nguyenmp/simplearchive/internal/subproc"
)

// OutputFile is the filename wget writes the page body to.
const OutputFile = "output.html"

// FaviconFile is the filename the favicon is written to.
const FaviconFile = "favicon.ico"

// Fetch downloads url and writes it to dir/output.html using wget. It returns
// the path of the written file.
func Fetch(ctx context.Context, pageURL, dir string) (string, error) {
	out := filepath.Join(dir, OutputFile)
	if _, err := subproc.Run(ctx, "", "wget",
		"--no-verbose",
		"--output-document="+out,
		pageURL,
	); err != nil {
		return "", fmt.Errorf("wget.Fetch: %w", err)
	}
	return out, nil
}

// faviconService is the base URL of the favicon lookup service.
const faviconService = "https://www.google.com/s2/favicons?domain="

// faviconURL builds the favicon-service URL for the host of pageURL.
func faviconURL(pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("parse url %q: %w", pageURL, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("parse url %q: no host", pageURL)
	}
	return faviconService + u.Hostname(), nil
}

// FetchFavicon downloads the favicon for the host of pageURL from Google's
// favicon service and writes it to dir/favicon.ico. It returns the path of the
// written file.
func FetchFavicon(ctx context.Context, pageURL, dir string) (string, error) {
	faviconURL, err := faviconURL(pageURL)
	if err != nil {
		return "", fmt.Errorf("wget.FetchFavicon: %w", err)
	}
	out := filepath.Join(dir, FaviconFile)
	if _, err := subproc.Run(ctx, "", "wget",
		"--no-verbose",
		"--output-document="+out,
		faviconURL,
	); err != nil {
		return "", fmt.Errorf("wget.FetchFavicon: %w", err)
	}
	return out, nil
}
