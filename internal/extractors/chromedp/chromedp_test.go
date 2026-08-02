//go:build chromedp

package chromedp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

func TestExtractor_writesScreenshotPDFDOM(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Hi</title></head><body><p>rendered</p></body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	steps, err := Extractor{Timeout: 30e9}.Run(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}
	for _, s := range steps {
		if s.Status != extractors.StatusSucceeded {
			t.Errorf("step %s status = %q, want succeeded: %v", s.Name, s.Status, s.Err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, ScreenshotFile)); err != nil || len(got) == 0 {
		t.Errorf("screenshot.png missing/empty: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, PDFFile)); err != nil || len(got) == 0 {
		t.Errorf("output.pdf missing/empty: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, DOMFile)); err != nil || len(got) == 0 {
		t.Errorf("dom_chromedp.html missing/empty: %v", err)
	}
}

// TestExtractor_remote mirrors the local test above but drives a remote Chrome
// via CHROME_CDP_URL (e.g. ws://sockpuppetbrowser:3000 against a
// sockpuppetbrowser container on a shared docker network). It is skipped when
// the env var is unset.
func TestExtractor_remote(t *testing.T) {
	t.Parallel()
	remote := os.Getenv("CHROME_CDP_URL")
	if remote == "" {
		t.Skip("CHROME_CDP_URL not set; skipping remote-browser test")
	}
	const body = "<html><head><title>Hi</title></head><body><p>rendered</p></body></html>"
	// Bind on all interfaces: the remote browser runs in another container and
	// reaches this server over the docker network by container hostname.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	srv.Listener = l
	srv.Start()
	defer srv.Close()

	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("hostname: %v", err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	pageURL := "http://" + host + ":" + port

	dir := t.TempDir()
	steps, err := Extractor{Timeout: 30e9, RemoteURL: remote}.Run(context.Background(), pageURL, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, s := range steps {
		if s.Status != extractors.StatusSucceeded {
			t.Errorf("step %s status = %q, want succeeded: %v", s.Name, s.Status, s.Err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, ScreenshotFile)); err != nil || len(got) == 0 {
		t.Errorf("screenshot.png missing/empty: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, PDFFile)); err != nil || len(got) == 0 {
		t.Errorf("output.pdf missing/empty: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, DOMFile)); err != nil || len(got) == 0 {
		t.Errorf("dom_chromedp.html missing/empty: %v", err)
	}
}

// TestExtractor_remoteProxyFailure verifies the proxy query-param contract:
// with a dead ProxyURL, the remote Chrome (launched by the CDP proxy with the
// --proxy-server flag from our query string) must fail to navigate. Skipped
// when CHROME_CDP_URL is unset.
func TestExtractor_remoteProxyFailure(t *testing.T) {
	t.Parallel()
	remote := os.Getenv("CHROME_CDP_URL")
	if remote == "" {
		t.Skip("CHROME_CDP_URL not set; skipping remote-browser test")
	}
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Listener = l
	srv.Start()
	defer srv.Close()

	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("hostname: %v", err)
	}
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	pageURL := "http://" + host + ":" + port

	steps, err := Extractor{Timeout: 15e9, RemoteURL: remote, ProxyURL: "socks5://127.0.0.1:1"}.Run(context.Background(), pageURL, t.TempDir())
	if err == nil {
		t.Fatal("want navigation failure through dead proxy, got nil error")
	}
	for _, s := range steps {
		if s.Status != extractors.StatusFailed {
			t.Errorf("step %s status = %q, want failed", s.Name, s.Status)
		}
	}
}

func TestRemoteWSURL(t *testing.T) {
	t.Parallel()

	t.Run("no proxy", func(t *testing.T) {
		got, err := remoteWSURL("ws://sockpuppetbrowser:3000", "")
		if err != nil {
			t.Fatalf("remoteWSURL: %v", err)
		}
		if got != "ws://sockpuppetbrowser:3000" {
			t.Errorf("got %q, want base URL unchanged", got)
		}
	})

	t.Run("proxy added, existing query preserved", func(t *testing.T) {
		got, err := remoteWSURL("ws://sockpuppetbrowser:3000/?headful=true", "socks5://tor-socks-proxy:9150")
		if err != nil {
			t.Fatalf("remoteWSURL: %v", err)
		}
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("parse result: %v", err)
		}
		q := u.Query()
		if q.Get("--proxy-server") != "socks5://tor-socks-proxy:9150" {
			t.Errorf("--proxy-server = %q", q.Get("--proxy-server"))
		}
		if q.Get("headful") != "true" {
			t.Errorf("headful = %q, want preserved", q.Get("headful"))
		}
	})

	t.Run("rejects non-ws scheme", func(t *testing.T) {
		if _, err := remoteWSURL("http://sockpuppetbrowser:3000", ""); err == nil {
			t.Error("want error for http:// scheme")
		}
	})
}
