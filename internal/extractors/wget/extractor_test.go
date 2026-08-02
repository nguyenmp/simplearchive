package wget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/extractors"
)

func TestHTMLExtractor_writesOutputHTML(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Hi</title></head><body>hello</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dir := t.TempDir()
	steps, err := HTMLExtractor{}.Run(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(steps))
	}
	step := steps[0]
	if step.Name != "dom" || step.Filename != OutputFile || step.Status != extractors.StatusSucceeded {
		t.Fatalf("step = %+v", step)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, OutputFile)); string(got) != body {
		t.Fatalf("output.html = %q, want %q", got, body)
	}
	if len(step.Cmd) == 0 || step.Cmd[0] != "wget" {
		t.Fatalf("Cmd = %v", step.Cmd)
	}
}

func TestHTMLExtractor_badURL_reportsFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	steps, err := HTMLExtractor{}.Run(context.Background(), "http://127.0.0.1:1/no-such-port", dir)
	if err == nil {
		t.Fatal("Run on unreachable URL returned nil error")
	}
	if len(steps) != 1 || steps[0].Status != extractors.StatusFailed {
		t.Fatalf("steps = %+v", steps)
	}
}

func TestFaviconExtractor_badURL_reportsFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	steps, _ := FaviconExtractor{}.Run(context.Background(), "not a url", dir)
	if len(steps) != 1 || steps[0].Status != extractors.StatusFailed {
		t.Fatalf("steps = %+v", steps)
	}
}
