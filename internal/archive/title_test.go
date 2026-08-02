package archive

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTitle_extractsTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		html string
		want string
	}{
		{"simple", `<html><head><title>Hello World</title></head></html>`, "Hello World"},
		{"whitespace", `<title>  Trim Me  </title>`, "Trim Me"},
		{"uppercase tag", `<TITLE>UPPER</TITLE>`, "UPPER"},
		{"mixed case", `<TiTlE>Mixed</TiTlE>`, "Mixed"},
		{"attributes", `<title id="x">With Attrs</title>`, "With Attrs"},
		{"none", `<html><body>no title here</body></html>`, ""},
		{"empty", `<title></title>`, ""},
		{"no close", `<title>unclosed`, ""},
		{"numeric entities", `<title>The rise of &#39;conspicuous waiting&#39;</title>`, "The rise of 'conspicuous waiting'"},
		{"named entities", `<title>Foo &amp; Bar &quot;quoted&quot;</title>`, `Foo & Bar "quoted"`},
		{"hex entities", `<title>It&#x27;s working</title>`, "It's working"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseTitle([]byte(tc.html)); got != tc.want {
				t.Errorf("ParseTitle(%q) = %q, want %q", tc.html, got, tc.want)
			}
		})
	}
}

func TestParseInfoJSONTitle_extractsTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		json string
		want string
	}{
		{"simple", `{"title":"My Video"}`, "My Video"},
		{"whitespace", `{"title":"  Trim Me  "}`, "Trim Me"},
		{"empty string", `{"title":""}`, ""},
		{"absent", `{"id":"abc123"}`, ""},
		{"invalid json", `{not json}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseInfoJSONTitle([]byte(tc.json)); got != tc.want {
				t.Errorf("ParseInfoJSONTitle(%q) = %q, want %q", tc.json, got, tc.want)
			}
		})
	}
}

func TestParseMercuryJSONTitle_extractsTitle(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		json string
		want string
	}{
		{"simple", `{"title":"My Article"}`, "My Article"},
		{"whitespace", `{"title":"  Trim Me  "}`, "Trim Me"},
		{"empty string", `{"title":""}`, ""},
		{"absent", `{"url":"https://example.com"}`, ""},
		{"invalid json", `{not json}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseMercuryJSONTitle([]byte(tc.json)); got != tc.want {
				t.Errorf("ParseMercuryJSONTitle(%q) = %q, want %q", tc.json, got, tc.want)
			}
		})
	}
}

func TestBestTitle_priority(t *testing.T) {
	t.Parallel()

	// Priority 1: yt-dlp info.json
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abc.info.json"), []byte(`{"title":"Info JSON"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "singlefile.html"), []byte(`<html><head><title>Singlefile</title></head></html>`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := BestTitle(dir); got != "Info JSON" {
		t.Errorf("BestTitle with info.json = %q, want Info JSON", got)
	}

	// Priority 2: mercury/article.json when no info.json
	dir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir2, "mercury"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "mercury", "article.json"), []byte(`{"title":"Mercury"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "singlefile.html"), []byte(`<html><head><title>Singlefile</title></head></html>`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := BestTitle(dir2); got != "Mercury" {
		t.Errorf("BestTitle with mercury = %q, want Mercury", got)
	}

	// Priority 3: singlefile.html when no info.json or mercury
	dir3 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir3, "singlefile.html"), []byte(`<html><head><title>Singlefile</title></head></html>`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir3, "output.html"), []byte(`<html><head><title>Output</title></head></html>`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := BestTitle(dir3); got != "Singlefile" {
		t.Errorf("BestTitle with singlefile = %q, want Singlefile", got)
	}

	// Priority 4: output.html when nothing else exists
	dir4 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir4, "output.html"), []byte(`<html><head><title>Output</title></head></html>`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := BestTitle(dir4); got != "Output" {
		t.Errorf("BestTitle with output = %q, want Output", got)
	}

	// Empty when nothing has a title
	dir5 := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir5, "output.html"), []byte(`<html><head></head></html>`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := BestTitle(dir5); got != "" {
		t.Errorf("BestTitle with no title = %q, want empty", got)
	}
}
