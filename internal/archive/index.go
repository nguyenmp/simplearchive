package archive

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nguyenmp/simplearchive/internal/extractors"
	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

// IndexFile is the filename of the per-snapshot index.
const IndexFile = "index.json"

// IndexData describes a snapshot to write to index.json. When Steps is
// non-empty, history/latest are built from it (keyed by Step.Name, with each
// Step's Cmd, Status, and timestamps). Otherwise Outputs is used as a
// convenience fallback that derives extractor names and commands from the
// output filenames (used by tests and simple fixtures).
type IndexData struct {
	Timestamp int64 // epoch microseconds
	URL       string
	Title     string // may be empty
	Dir       string // snapshot directory (for pwd in history entries)
	Outputs   []string
	Steps     []extractors.Step
}

// IndexEntry is the subset of an on-disk index.json that simplearchive imports
// into meta.db. Title is the empty string when the index's "title" field is
// null or absent.
type IndexEntry struct {
	Timestamp  int64 // epoch microseconds
	URL        string
	Title      string
	IsArchived bool
}

// archiveResult is a single entry in the "history" map, matching ArchiveBox's
// ArchiveResult schema (subset of fields).
type archiveResult struct {
	Cmd     []string  `json:"cmd"`
	Output  string    `json:"output"`
	Pwd     string    `json:"pwd"`
	Schema  string    `json:"schema"`
	StartTs time.Time `json:"start_ts"`
	EndTs   time.Time `json:"end_ts"`
	Status  string    `json:"status"`
}

type linkJSON struct {
	Schema     string                     `json:"schema"`
	URL        string                     `json:"url"`
	Timestamp  string                     `json:"timestamp"`
	Title      *string                    `json:"title"`
	Tags       *[]string                  `json:"tags"`
	TagsStr    string                     `json:"tags_str"`
	Sources    []string                   `json:"sources"`
	IsArchived bool                       `json:"is_archived"`
	IsStatic   bool                       `json:"is_static"`
	Domain     string                     `json:"domain"`
	BaseURL    string                     `json:"base_url"`
	Scheme     string                     `json:"scheme"`
	Path       string                     `json:"path"`
	Latest     map[string]string          `json:"latest"`
	History    map[string][]archiveResult `json:"history"`
}

// buildHistory constructs the history and latest maps from IndexData. When
// Steps is non-empty, history/latest are built from it (keyed by Step.Name).
// Otherwise Outputs is used as a fallback that derives extractor names from
// filenames. A title entry is added when the title is non-empty.
func buildHistory(data IndexData, u *url.URL) (map[string][]archiveResult, map[string]string) {
	latest := map[string]string{}
	history := map[string][]archiveResult{}
	now := time.Now().UTC()

	if len(data.Steps) > 0 {
		for _, step := range data.Steps {
			cmd := step.Cmd
			if cmd == nil {
				cmd = []string{}
			}
			latest[step.Name] = step.Filename
			history[step.Name] = []archiveResult{{
				Cmd:     cmd,
				Output:  step.Filename,
				Pwd:     data.Dir,
				Schema:  "ArchiveResult",
				StartTs: step.StartTs.UTC(),
				EndTs:   step.EndTs.UTC(),
				Status:  step.Status,
			}}
		}
	} else {
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
	}

	title, hasTitle := titleOrEmpty(data.Title)
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

	return history, latest
}

// WriteIndex writes an ArchiveBox-compatible per-snapshot index.json into dir.
func WriteIndex(data IndexData) error {
	u, err := url.Parse(data.URL)
	if err != nil {
		return fmt.Errorf("archive.WriteIndex: parse url: %w", err)
	}

	history, latest := buildHistory(data, u)

	title, hasTitle := titleOrEmpty(data.Title)
	var titlePtr *string
	if hasTitle {
		titlePtr = &title
	}

	link := linkJSON{
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

	jsonBytes, err := json.MarshalIndent(link, "", "    ")
	if err != nil {
		return fmt.Errorf("archive.WriteIndex: marshal: %w", err)
	}
	path := filepath.Join(data.Dir, IndexFile)
	if err := os.WriteFile(path, append(jsonBytes, '\n'), extractors.DefaultFilePerm); err != nil {
		return fmt.Errorf("archive.WriteIndex: write %q: %w", path, err)
	}
	return nil
}

// extractorFor maps an output filename to its ArchiveBox extractor/plugin name.
func extractorFor(filename string) string {
	switch filename {
	case "output.html":
		return "dom"
	case "curl.html":
		return "curl"
	case "favicon.ico":
		return "favicon"
	case "headers.json":
		return "headers"
	case "singlefile.html":
		return "singlefile"
	case "singlefile_proxy.html":
		return "singlefile_proxy"
	case "screenshot_proxy.png":
		return "screenshot_proxy"
	case "output_proxy.pdf":
		return "pdf_proxy"
	case "dom_chromedp_proxy.html":
		return "chromedp_dom_proxy"
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

// baseURL returns the scheme://host portion of u, matching ArchiveBox's
// base_url field (scheme://domain, which excludes the port).
func baseURL(u *url.URL) string {
	if u.Hostname() == "" {
		return ""
	}
	return u.Scheme + "://" + u.Hostname()
}

// ReadIndex decodes a single snapshot's index.json into an IndexEntry. The
// timestamp is parsed from ArchiveBox's "seconds.microseconds" string form
// into epoch microseconds. A null or absent title becomes the empty string.
func ReadIndex(path string) (IndexEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return IndexEntry{}, fmt.Errorf("archive.ReadIndex: read %q: %w", path, err)
	}
	var link linkJSON
	if err := json.Unmarshal(raw, &link); err != nil {
		return IndexEntry{}, fmt.Errorf("archive.ReadIndex: unmarshal %q: %w", path, err)
	}
	ts, err := snapshot.Parse(link.Timestamp)
	if err != nil {
		return IndexEntry{}, fmt.Errorf("archive.ReadIndex: %q: %w", path, err)
	}
	entry := IndexEntry{
		Timestamp:  ts,
		URL:        link.URL,
		IsArchived: link.IsArchived,
	}
	if link.Title != nil {
		entry.Title = *link.Title
	}
	return entry, nil
}

// Scan walks root looking for per-snapshot index.json files (root/<timestamp>/
// index.json) and returns one IndexEntry per snapshot, sorted by timestamp
// ascending. Directories without an index.json are skipped (logged at debug).
// It returns an error if root itself cannot be globbed.
func Scan(root string) ([]IndexEntry, error) {
	pattern := filepath.Join(root, "*", IndexFile)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("archive.Scan: glob %q: %w", pattern, err)
	}
	entries := make([]IndexEntry, 0, len(matches))
	for _, indexPath := range matches {
		entry, err := ReadIndex(indexPath)
		if err != nil {
			slog.Debug("archive.Scan: skipping unreadable index", "path", indexPath, "err", err)
			continue
		}
		if entry.Title == "" {
			if t := BestTitle(filepath.Dir(indexPath)); t != "" {
				entry.Title = t
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
	return entries, nil
}
