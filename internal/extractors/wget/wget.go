// Package wget wraps the wget command-line tool to fetch a URL into a snapshot
// directory.
package wget

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
)

// OutputFile is the filename wget writes the page body to.
const OutputFile = "output.html"

// FaviconFile is the filename the favicon is written to.
const FaviconFile = "favicon.ico"

// Fetch downloads url and writes it to dir/output.html using wget. It returns
// the path of the written file.
func Fetch(ctx context.Context, url, dir string) (string, error) {
	out := filepath.Join(dir, OutputFile)
	cmd := exec.CommandContext(ctx, "wget",
		"--no-verbose",
		"--output-document="+out,
		url,
	)
	if stderr, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("wget.Fetch: %w: %s", err, stderr)
	}
	return out, nil
}

// faviconService is the base URL of the favicon lookup service.
const faviconService = "https://www.google.com/s2/favicons?domain="

// faviconURL builds the favicon-service URL for the host of pageURL.
func faviconURL(pageURL string) (string, error) {
	u, err := url.Parse(pageURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("parse url %q: %w", pageURL, err)
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
	cmd := exec.CommandContext(ctx, "wget",
		"--no-verbose",
		"--output-document="+out,
		faviconURL,
	)
	if stderr, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("wget.FetchFavicon: %w: %s", err, stderr)
	}
	return out, nil
}
