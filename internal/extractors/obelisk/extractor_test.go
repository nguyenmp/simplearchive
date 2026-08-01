package obelisk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

func TestExtractor_writesSingleFileHTML(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Hi</title></head><body><p>hello world</p></body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	steps, err := Extractor{}.Run(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	s := steps[0]
	if s.Name != "singlefile" || s.Filename != OutputFile || s.Status != extractors.StatusSucceeded {
		t.Fatalf("step = %+v", s)
	}
	got, err := os.ReadFile(filepath.Join(dir, OutputFile))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(got), "hello world") {
		t.Fatalf("singlefile.html = %q, want it to contain the page text", got)
	}
}

func TestExtractor_badURL_reportsFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	steps, err := Extractor{}.Run(context.Background(), "http://127.0.0.1:1/no-such-port", dir)
	if err == nil {
		t.Fatal("Run on unreachable URL returned nil error")
	}
	if len(steps) != 1 || steps[0].Status != extractors.StatusFailed {
		t.Fatalf("steps = %+v", steps)
	}
}
