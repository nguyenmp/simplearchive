package archive

import (
	"bytes"
	"encoding/json"
	"html"
	"strings"
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
