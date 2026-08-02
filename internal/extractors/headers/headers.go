// Package headers fetches the HTTP response headers for a URL and writes them
// to dir/headers.json in a format compatible with ArchiveBox's headers output.
package headers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// OutputFile is the filename the headers are written to.
const OutputFile = "headers.json"

// Fetch performs a HEAD request against url and writes the response status,
// final URL, and headers to dir/headers.json. It returns the path of the
// written file.
func Fetch(ctx context.Context, pageURL, dir string) (string, error) {
	return FetchWithClient(ctx, pageURL, dir, &http.Client{Timeout: 60 * time.Second})
}

// FetchWithClient is like Fetch but uses the supplied *http.Client, allowing
// callers to inject a custom transport (e.g. a SOCKS5 proxy).
func FetchWithClient(ctx context.Context, pageURL, dir string, client *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, pageURL, nil)
	if err != nil {
		return "", fmt.Errorf("headers.Fetch: build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("headers.Fetch: request %q: %w", pageURL, err)
	}
	defer resp.Body.Close()

	return writeHeaders(resp, dir)
}

func writeHeaders(resp *http.Response, dir string) (string, error) {
	out := map[string]any{
		"URL":         resp.Request.URL.String(),
		"Status-Code": resp.StatusCode,
	}
	for k, vs := range resp.Header {
		if len(vs) == 0 {
			continue
		}
		out[k] = vs[0]
	}

	data, err := json.MarshalIndent(out, "", "    ")
	if err != nil {
		return "", fmt.Errorf("headers.writeHeaders: marshal: %w", err)
	}
	path := filepath.Join(dir, OutputFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("headers.writeHeaders: write %q: %w", path, err)
	}
	return path, nil
}
