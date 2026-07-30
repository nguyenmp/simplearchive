package headers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetch_writesHeadersJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Custom", "hello")
		w.WriteHeader(200)
	}))
	defer srv.Close()

	dir := t.TempDir()
	out, err := Fetch(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if filepath.Base(out) != OutputFile {
		t.Fatalf("out = %q, want base %q", out, OutputFile)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got["Status-Code"] != float64(200) {
		t.Fatalf("Status-Code = %v, want 200", got["Status-Code"])
	}
	if got["Content-Type"] != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %v, want text/html; charset=utf-8", got["Content-Type"])
	}
	if got["X-Custom"] != "hello" {
		t.Fatalf("X-Custom = %v, want hello", got["X-Custom"])
	}
	if got["URL"] != srv.URL {
		t.Fatalf("URL = %v, want %v", got["URL"], srv.URL)
	}
}

func TestFetch_badURL_returnsError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := Fetch(context.Background(), "http://127.0.0.1:1/no-such-port", dir)
	if err == nil {
		t.Fatal("Fetch on unreachable URL returned nil error")
	}
}
