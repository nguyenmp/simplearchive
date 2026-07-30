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
