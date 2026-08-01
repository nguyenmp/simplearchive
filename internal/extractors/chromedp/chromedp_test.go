//go:build chromedp

package chromedp

import (
	"context"
	"net/http"
	"net/http/httptest"
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
