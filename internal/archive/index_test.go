package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

func TestWriteIndex_writesLinkSchema(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := IndexData{
		Timestamp: 1728277530511,
		URL:       "https://example.com/path/page",
		Title:     "Example Page",
		Dir:       dir,
		Outputs:   []string{"output.html", "favicon.ico", "headers.json"},
	}
	if err := WriteIndex(data); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	path := filepath.Join(dir, IndexFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var got linkJSON
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Schema != "Link" {
		t.Errorf("schema = %q, want Link", got.Schema)
	}
	if got.URL != "https://example.com/path/page" {
		t.Errorf("url = %q", got.URL)
	}
	if got.Timestamp != snapshot.Format(1728277530511) {
		t.Errorf("timestamp = %q, want %q", got.Timestamp, snapshot.Format(1728277530511))
	}
	if got.Title == nil || *got.Title != "Example Page" {
		t.Errorf("title = %v, want Example Page", got.Title)
	}
	if got.Domain != "example.com" {
		t.Errorf("domain = %q, want example.com", got.Domain)
	}
	if got.Scheme != "https" {
		t.Errorf("scheme = %q, want https", got.Scheme)
	}
	if got.BaseURL != "https://example.com" {
		t.Errorf("base_url = %q, want https://example.com", got.BaseURL)
	}
	if !got.IsArchived {
		t.Error("is_archived = false, want true")
	}
	if got.Latest["dom"] != "output.html" {
		t.Errorf("latest.dom = %q, want output.html", got.Latest["dom"])
	}
	if got.Latest["favicon"] != "favicon.ico" {
		t.Errorf("latest.favicon = %q, want favicon.ico", got.Latest["favicon"])
	}
	if got.Latest["headers"] != "headers.json" {
		t.Errorf("latest.headers = %q, want headers.json", got.Latest["headers"])
	}
	if got.Latest["title"] != "Example Page" {
		t.Errorf("latest.title = %q, want Example Page", got.Latest["title"])
	}
	if h, ok := got.History["favicon"]; !ok || len(h) != 1 || h[0].Output != "favicon.ico" {
		t.Errorf("history.favicon = %v, want one entry with output favicon.ico", got.History["favicon"])
	}

	// ArchiveBox's ArchiveResult schema requires cmd to be a list (never null)
	// and every element to be a non-empty string.
	for method, results := range got.History {
		if len(results) != 1 {
			t.Fatalf("history.%s has %d entries, want 1", method, len(results))
		}
		r := results[0]
		if r.Cmd == nil {
			t.Errorf("history.%s.cmd = nil, want non-nil list", method)
			continue
		}
		for _, arg := range r.Cmd {
			if arg == "" {
				t.Errorf("history.%s.cmd contains empty arg", method)
			}
		}
	}
	if dom, ok := got.History["dom"]; !ok || len(dom) != 1 || dom[0].Cmd[0] != "wget" {
		t.Errorf("history.dom.cmd = %v, want wget invocation", got.History["dom"])
	}
}

func TestWriteIndex_nullTitleWhenEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := IndexData{
		Timestamp: 1700000000000,
		URL:       "https://example.com",
		Dir:       dir,
		Outputs:   []string{"output.html"},
	}
	if err := WriteIndex(data); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, IndexFile))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got linkJSON
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Title != nil {
		t.Errorf("title = %v, want nil", got.Title)
	}
	if _, ok := got.Latest["title"]; ok {
		t.Error("latest.title present, want absent for empty title")
	}
}

func TestReadIndex_decodesABSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := IndexData{
		Timestamp: 1728277530511,
		URL:       "https://example.com/path",
		Title:      "Example Page",
		Dir:        dir,
		Outputs:    []string{"output.html", "favicon.ico", "headers.json"},
	}
	if err := WriteIndex(data); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	got, err := ReadIndex(filepath.Join(dir, IndexFile))
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if got.Timestamp != 1728277530511 {
		t.Errorf("timestamp = %d, want 1728277530511", got.Timestamp)
	}
	if got.URL != "https://example.com/path" {
		t.Errorf("url = %q", got.URL)
	}
	if got.Title != "Example Page" {
		t.Errorf("title = %q, want Example Page", got.Title)
	}
	if !got.IsArchived {
		t.Error("is_archived = false, want true")
	}
}

func TestReadIndex_nullTitleBecomesEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := WriteIndex(IndexData{
		Timestamp: 1700000000000,
		URL:       "https://example.com",
		Dir:       dir,
		Outputs:   []string{"output.html"},
	}); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	got, err := ReadIndex(filepath.Join(dir, IndexFile))
	if err != nil {
		t.Fatalf("ReadIndex: %v", err)
	}
	if got.Title != "" {
		t.Errorf("title = %q, want empty", got.Title)
	}
}

func TestReadIndex_missingFileErrors(t *testing.T) {
	t.Parallel()
	if _, err := ReadIndex(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("ReadIndex on missing file returned nil error")
	}
}

func TestScan_collectsAndSortsSnapshots(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// Two snapshots written out of order; Scan must return them sorted by ts.
	for _, data := range []IndexData{
		{Timestamp: 1700000000002, URL: "https://b.example.com", Title: "B", Dir: filepath.Join(root, snapshot.Format(1700000000002)), Outputs: []string{"output.html"}},
		{Timestamp: 1700000000001, URL: "https://a.example.com", Title: "A", Dir: filepath.Join(root, snapshot.Format(1700000000001)), Outputs: []string{"output.html"}},
	} {
		if err := os.MkdirAll(data.Dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := WriteIndex(data); err != nil {
			t.Fatalf("WriteIndex: %v", err)
		}
	}
	got, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(got))
	}
	if got[0].Timestamp != 1700000000001 || got[0].URL != "https://a.example.com" {
		t.Errorf("entries[0] = %+v, want A", got[0])
	}
	if got[1].Timestamp != 1700000000002 || got[1].URL != "https://b.example.com" {
		t.Errorf("entries[1] = %+v, want B", got[1])
	}
}

func TestScan_skipsDirsWithoutIndex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// A stray directory with no index.json should be ignored, not error.
	if err := os.MkdirAll(filepath.Join(root, "stray-no-index"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, snapshot.Format(1700000000000))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteIndex(IndexData{
		Timestamp: 1700000000000,
		URL:       "https://example.com",
		Title:      "One",
		Dir:        dir,
		Outputs:    []string{"output.html"},
	}); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	got, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 1 || got[0].URL != "https://example.com" {
		t.Fatalf("entries = %+v, want one example.com", got)
	}
}
