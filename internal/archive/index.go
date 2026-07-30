package archive

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// IndexFile is the filename of the per-snapshot index.
const IndexFile = "index.json"

// IndexData describes a snapshot to write to index.json.
type IndexData struct {
	Timestamp int64  // epoch ms
	URL       string
	Title     string // may be empty
	Dir       string // snapshot directory (for pwd in history entries)
	Outputs   []string
}

// archiveResult is a single entry in the "history" map, matching ArchiveBox's
// ArchiveResult schema (subset of fields).
type archiveResult struct {
	Cmd      []string  `json:"cmd"`
	Output   string    `json:"output"`
	Pwd      string    `json:"pwd"`
	Schema   string    `json:"schema"`
	StartTs  time.Time `json:"start_ts"`
	EndTs    time.Time `json:"end_ts"`
	Status   string    `json:"status"`
}

type linkJSON struct {
	Schema     string            `json:"schema"`
	URL        string            `json:"url"`
	Timestamp  string            `json:"timestamp"`
	Title      *string           `json:"title"`
	Tags       *[]string         `json:"tags"`
	TagsStr    string            `json:"tags_str"`
	Sources    []string          `json:"sources"`
	IsArchived bool             `json:"is_archived"`
	IsStatic   bool             `json:"is_static"`
	Domain     string            `json:"domain"`
	BaseURL    string            `json:"base_url"`
	Scheme     string            `json:"scheme"`
	Path       string            `json:"path"`
	Latest     map[string]string `json:"latest"`
	History    map[string][]archiveResult `json:"history"`
}

// WriteIndex writes an ArchiveBox-compatible per-snapshot index.json into dir.
func WriteIndex(data IndexData) error {
	u, err := url.Parse(data.URL)
	if err != nil {
		return fmt.Errorf("archive.WriteIndex: parse url: %w", err)
	}

	now := time.Now().UTC()
	title, hasTitle := titleOrEmpty(data.Title)

	latest := map[string]string{}
	history := map[string][]archiveResult{}
	for _, out := range data.Outputs {
		extractor := extractorFor(out)
		latest[extractor] = out
		history[extractor] = []archiveResult{{
			Cmd:     commandFor(extractor, data, u),
			Output:  out,
			Pwd:     data.Dir,
			Schema:  "ArchiveResult",
			StartTs: now,
			EndTs:   now,
			Status:  "succeeded",
		}}
	}
	if hasTitle {
		latest["title"] = title
		history["title"] = []archiveResult{{
			Cmd:     commandFor("title", data, u),
			Output:  title,
			Pwd:     data.Dir,
			Schema:  "ArchiveResult",
			StartTs: now,
			EndTs:   now,
			Status:  "succeeded",
		}}
	}

	var titlePtr *string
	if hasTitle {
		titlePtr = &title
	}

	doc := linkJSON{
		Schema:     "Link",
		URL:        data.URL,
		Timestamp:  snapshot.Format(data.Timestamp),
		Title:      titlePtr,
		Tags:       nil,
		Sources:    []string{},
		IsArchived: true,
		IsStatic:   false,
		Domain:     u.Hostname(),
		BaseURL:    baseURL(u),
		Scheme:     u.Scheme,
		Path:       u.Path,
		Latest:     latest,
		History:    history,
	}

	enc, err := json.MarshalIndent(doc, "", "    ")
	if err != nil {
		return fmt.Errorf("archive.WriteIndex: marshal: %w", err)
	}
	path := filepath.Join(data.Dir, IndexFile)
	if err := os.WriteFile(path, append(enc, '\n'), 0o644); err != nil {
		return fmt.Errorf("archive.WriteIndex: write %q: %w", path, err)
	}
	return nil
}

// extractorFor maps an output filename to its ArchiveBox extractor/plugin name.
func extractorFor(filename string) string {
	switch filename {
	case "output.html":
		return "dom"
	case "favicon.ico":
		return "favicon"
	case "headers.json":
		return "headers"
	default:
		return filename
	}
}

// commandFor returns the shell command list recorded in an ArchiveResult's
// cmd field for an extractor. Extractors that do not run a shell command
// (headers via net/http, title via HTML parsing) get an empty list, which
// ArchiveBox's schema accepts. The wget-based extractors record the actual
// invocation so the snapshot is debuggable and reimportable.
func commandFor(extractor string, data IndexData, u *url.URL) []string {
	switch extractor {
	case "dom":
		return []string{"wget", "--no-verbose", "--output-document=" + filepath.Join(data.Dir, "output.html"), data.URL}
	case "favicon":
		return []string{"wget", "--no-verbose", "--output-document=" + filepath.Join(data.Dir, "favicon.ico"), "https://www.google.com/s2/favicons?domain=" + u.Hostname()}
	default:
		return []string{}
	}
}

// baseURL returns the scheme://host[:port] portion of u, matching ArchiveBox's
// base_url field.
func baseURL(u *url.URL) string {
	if u.Host == "" {
		return ""
	}
	if u.Port() == "" {
		return u.Scheme + "://" + u.Host
	}
	return u.Scheme + "://" + u.Host
}
