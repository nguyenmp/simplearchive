package archive

import (
	"bytes"
	"strings"
)

// ParseTitle extracts the contents of the first <title>...</title> element
// from an HTML document. It returns an empty string if no title is found.
func ParseTitle(html []byte) string {
	lower := bytes.ToLower(html)
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
	return strings.TrimSpace(string(html[contentStart : contentStart+end]))
}

// titleOrEmpty trims the title and reports whether it was non-empty.
func titleOrEmpty(title string) (string, bool) {
	title = strings.TrimSpace(title)
	return title, title != ""
}
