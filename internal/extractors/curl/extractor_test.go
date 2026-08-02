package curl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

func TestExtractor_success(t *testing.T) {
	t.Parallel()
	// curl(1) is available in the dev image; run it against a known-good URL.
	// Use example.com which is reliably online.
	ctx := context.Background()
	dir := t.TempDir()
	steps, err := Extractor{}.Run(ctx, "https://example.com", dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	s := steps[0]
	if s.Name != "curl" || s.Filename != OutputFile || s.Status != extractors.StatusSucceeded {
		t.Fatalf("step = %+v", s)
	}
	if _, err := os.Stat(filepath.Join(dir, OutputFile)); err != nil {
		t.Fatalf("curl.html not written: %v", err)
	}
}

func TestExtractor_withProxy(t *testing.T) {
	t.Parallel()
	// Even with a bogus proxy, Run should try direct first and succeed for
	// a real URL. (The proxy fallback is only hit when direct fails.)
	ctx := context.Background()
	dir := t.TempDir()
	steps, err := Extractor{ProxyURL: "socks5://127.0.0.1:1"}.Run(ctx, "https://example.com", dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	if steps[0].Status != extractors.StatusSucceeded {
		t.Fatalf("step = %+v", steps[0])
	}
}
