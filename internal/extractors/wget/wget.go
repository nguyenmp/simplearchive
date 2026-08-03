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

// wgetArgv returns the full wget argv (program name included) that downloads
// url to outputPath. It is shared by Fetch and each step's recorded Cmd so the
// command recorded in index.json always matches what was actually executed.
func wgetArgv(outputPath, url string) []string {
	return []string{"wget", "--no-verbose", "--output-document=" + outputPath, url}
}

// runWget executes a full wget argv (as built by wgetArgv) via subproc.Run,
// which takes the program name and its args separately.
func runWget(ctx context.Context, dir string, argv []string) error {
	_, err := subproc.Run(ctx, dir, argv[0], argv[1:]...)
	return err
}

// Fetch downloads url and writes it to dir/output.html using wget. It returns
// the path of the written file.
func Fetch(ctx context.Context, pageURL, dir string) (string, error) {
	out := filepath.Join(dir, OutputFile)
	if err := runWget(ctx, "", wgetArgv(out, pageURL)); err != nil {
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

// faviconArgv returns the full wget argv that fetches the favicon for pageURL
// into dir/favicon.ico. It errors when pageURL has no host to look up. Shared
// by FetchFavicon and the favicon step's recorded Cmd so they cannot drift.
func faviconArgv(pageURL, dir string) ([]string, error) {
	serviceURL, err := faviconURL(pageURL)
	if err != nil {
		return nil, err
	}
	return wgetArgv(filepath.Join(dir, FaviconFile), serviceURL), nil
}

// FetchFavicon downloads the favicon for the host of pageURL from Google's
// favicon service and writes it to dir/favicon.ico. It returns the path of the
// written file.
func FetchFavicon(ctx context.Context, pageURL, dir string) (string, error) {
	argv, err := faviconArgv(pageURL, dir)
	if err != nil {
		return "", fmt.Errorf("wget.FetchFavicon: %w", err)
	}
	out := filepath.Join(dir, FaviconFile)
	if err := runWget(ctx, "", argv); err != nil {
		return "", fmt.Errorf("wget.FetchFavicon: %w", err)
	}
	return out, nil
}
