package archive

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/nguyenmp/simplearchive/internal/extractors/curl"
	"github.com/nguyenmp/simplearchive/internal/extractors/obelisk"
	"github.com/nguyenmp/simplearchive/internal/extractors/obeliskproxy"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
)

// ParseTitle extracts the contents of the first <title>...</title> element
// from an HTML document. It returns an empty string if no title is found.
func ParseTitle(doc []byte) string {
	lower := bytes.ToLower(doc)
	start := bytes.Index(lower, []byte("<title"))
	if start < 0 {
		return ""
	}
	gt := bytes.IndexByte(lower[start:], '>')
	if gt < 0 {
		return ""
	}
	contentStart := start + gt + 1
	end := bytes.Index(lower[contentStart:], []byte("</title>"))
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(string(doc[contentStart : contentStart+end])))
}

// titleOrEmpty trims the title and reports whether it was non-empty.
func titleOrEmpty(title string) (string, bool) {
	title = strings.TrimSpace(title)
	return title, title != ""
}

// ParseInfoJSONTitle extracts the "title" field from a yt-dlp info JSON
// document. It returns an empty string if the field is absent, empty, or the
// data is not valid JSON.
func ParseInfoJSONTitle(data []byte) string {
	var v struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	return strings.TrimSpace(v.Title)
}

// ParseMercuryJSONTitle extracts the "title" field from a mercury/article.json
// document. It returns an empty string if the field is absent, empty, or the
// data is not valid JSON.
func ParseMercuryJSONTitle(data []byte) string {
	var v struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	return strings.TrimSpace(v.Title)
}

// BestTitle returns the best available title for the snapshot in dir, trying
// sources in priority order: yt-dlp info.json, mercury/article.json,
// obelisk singlefile HTML, then wget DOM HTML. The first non-empty title
// wins; empty is returned when no source has a title.
func BestTitle(dir string) string {
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.info.json")); len(matches) > 0 {
		if data, err := os.ReadFile(matches[0]); err == nil {
			if t := ParseInfoJSONTitle(data); t != "" {
				return t
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "mercury", "article.json")); err == nil {
		if t := ParseMercuryJSONTitle(data); t != "" {
			return t
		}
	}
	if html, err := os.ReadFile(filepath.Join(dir, obelisk.OutputFile)); err == nil {
		if t := ParseTitle(html); t != "" {
			return t
		}
	}
	if html, err := os.ReadFile(filepath.Join(dir, obeliskproxy.OutputFile)); err == nil {
		if t := ParseTitle(html); t != "" {
			return t
		}
	}
	if html, err := os.ReadFile(filepath.Join(dir, wget.OutputFile)); err == nil {
		if t := ParseTitle(html); t != "" {
			return t
		}
	}
	if html, err := os.ReadFile(filepath.Join(dir, curl.OutputFile)); err == nil {
		if t := ParseTitle(html); t != "" {
			return t
		}
	}
	return ""
}
