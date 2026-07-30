package wget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetch_writesOutputHTML(t *testing.T) {
	t.Parallel()
	const body = "<html><head><title>Hi</title></head><body>hello</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
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
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != body {
		t.Fatalf("output.html = %q, want %q", got, body)
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

func TestFaviconURL_buildsGoogleServiceURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"https://example.com/path", "https://www.google.com/s2/favicons?domain=example.com"},
		{"https://youtu.be/j7P5szP95nI", "https://www.google.com/s2/favicons?domain=youtu.be"},
		{"https://www.example.com:8080/x", "https://www.google.com/s2/favicons?domain=www.example.com"},
	}
	for _, tc := range cases {
		got, err := faviconURL(tc.in)
		if err != nil {
			t.Fatalf("faviconURL(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("faviconURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFaviconURL_rejectsInvalidURL(t *testing.T) {
	t.Parallel()
	if _, err := faviconURL("not a url"); err == nil {
		t.Fatal("faviconURL on invalid URL returned nil error")
	}
}
