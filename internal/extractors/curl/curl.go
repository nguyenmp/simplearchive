// Package curl wraps the curl command-line tool to fetch a URL into a
// snapshot directory. It tries a direct request first and falls back to a
// SOCKS5 proxy when configured and the direct request fails.
package curl

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/nguyenmp/simplearchive/internal/subproc"
)

// OutputFile is the filename curl writes the page body to.
const OutputFile = "curl.html"

// Fetch downloads url and writes it to dir/curl.html using curl. proxyURL
// is an optional socks5:// URL; when non-empty curl is invoked with
// --socks5-hostname. It returns the written file path.
func Fetch(ctx context.Context, pageURL, dir, proxyURL string) (string, error) {
	out := filepath.Join(dir, OutputFile)
	argv := fetchArgv(pageURL, out, proxyURL)
	if _, err := subproc.Run(ctx, "", "curl", argv...); err != nil {
		transport := "direct"
		if proxyURL != "" {
			transport = "proxy"
		}
		return "", fmt.Errorf("curl.Fetch (%s) %q: %w", transport, pageURL, err)
	}
	return out, nil
}

func fetchArgv(pageURL, out, proxyURL string) []string {
	argv := []string{"-sSL", "--fail", "-o", out, pageURL}
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err == nil && u.Host != "" {
			argv = append([]string{"--socks5-hostname", u.Host}, argv...)
		}
	}
	return argv
}
