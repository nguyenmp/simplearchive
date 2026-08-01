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
	if len(data.Steps) > 0 {
		for _, s := range data.Steps {
			cmd := s.Cmd
			if cmd == nil {
				cmd = []string{}
			}
			latest[s.Name] = s.Filename
			history[s.Name] = []archiveResult{{
				Cmd:     cmd,
				Output:  s.Filename,
				Pwd:     data.Dir,
				Schema:  "ArchiveResult",
				StartTs: s.StartTs.UTC(),
				EndTs:   s.EndTs.UTC(),
				Status:  s.Status,
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
	case "singlefile.html":
		return "singlefile"
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

// ReadIndex decodes a single snapshot's index.json into an IndexEntry. The
// timestamp is parsed from ArchiveBox's "seconds.microseconds" string form
// into epoch microseconds. A null or absent title becomes the empty string.
func ReadIndex(path string) (IndexEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return IndexEntry{}, fmt.Errorf("archive.ReadIndex: read %q: %w", path, err)
	}
	var doc linkJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return IndexEntry{}, fmt.Errorf("archive.ReadIndex: unmarshal %q: %w", path, err)
	}
	ts, err := snapshot.Parse(doc.Timestamp)
	if err != nil {
		return IndexEntry{}, fmt.Errorf("archive.ReadIndex: %q: %w", path, err)
	}
	entry := IndexEntry{
		Timestamp:  ts,
		URL:        doc.URL,
		IsArchived: doc.IsArchived,
	}
	if doc.Title != nil {
		entry.Title = *doc.Title
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
	for _, p := range matches {
		entry, err := ReadIndex(p)
		if err != nil {
			slog.Debug("archive.Scan: skipping unreadable index", "path", p, "err", err)
			continue
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp < entries[j].Timestamp
	})
	return entries, nil
}
