package headers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

func TestExtractor_writesHeadersJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "hello")
		w.WriteHeader(200)
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
	if s.Name != "headers" || s.Filename != OutputFile || s.Status != extractors.StatusSucceeded {
		t.Fatalf("step = %+v", s)
	}
	if s.Cmd != nil {
		t.Fatalf("Cmd = %v, want nil (in-process)", s.Cmd)
	}
	if _, err := os.Stat(filepath.Join(dir, OutputFile)); err != nil {
		t.Fatalf("headers.json not written: %v", err)
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
