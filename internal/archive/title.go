package archive

import (
	"bytes"
	"encoding/json"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nguyenmp/simplearchive/internal/extractors/curl"
	"github.com/nguyenmp/simplearchive/internal/extractors/obelisk"
	"github.com/nguyenmp/simplearchive/internal/extractors/obeliskproxy"
	"github.com/nguyenmp/simplearchive/internal/extractors/wget"
	"golang.org/x/net/publicsuffix"
)

// ParseTitle extracts the contents of the first <title>...</title> element
// from an HTML document. It returns an empty string if no title is found.
func ParseTitle(doc []byte) string {
	lower := bytes.ToLower(doc)
	start := bytes.Index(lower, []byte("<title"))
	if start < 0 {
		return ""
	}
	closeAngleIdx := bytes.IndexByte(lower[start:], '>')
	if closeAngleIdx < 0 {
		return ""
	}
	contentStart := start + closeAngleIdx + 1
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

// ParseInfoJSON extracts the "title", "webpage_url", and "extractor" fields
// from a yt-dlp info JSON document. Empty strings are returned for absent
// fields or invalid JSON.
func ParseInfoJSON(data []byte) (title, webpageURL, extractor string) {
	var info struct {
		Title      string `json:"title"`
		WebpageURL string `json:"webpage_url"`
		Extractor  string `json:"extractor"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return "", "", ""
	}
	return strings.TrimSpace(info.Title), strings.TrimSpace(info.WebpageURL), strings.TrimSpace(info.Extractor)
}

// domainAliases maps single-purpose shortener domains to the registered
// domain of the site they alias, so e.g. a youtu.be link is the same site as
// the youtube.com page yt-dlp canonicalizes it to. Generic shorteners used
// by arbitrary destinations (bit.ly, ...) must not be listed here.
var domainAliases = map[string]string{
	"youtu.be":   "youtube.com",
	"instagr.am": "instagram.com",
}

// sameSite reports whether a and b are URLs on the same site: equal
// hostnames or equal registered domains (eTLD+1, after applying
// domainAliases), case-insensitive. This treats "m.youtube.com" and
// "www.youtube.com" as the same site while distinguishing a page from media
// embedded from another domain. It returns false when either URL fails to
// parse or has no hostname, so an absent webpage_url never matches.
func sameSite(a, b string) bool {
	urlA, urlParseErrA := url.Parse(a)
	urlB, urlParseErrB := url.Parse(b)
	if urlParseErrA != nil || urlParseErrB != nil {
		return false
	}
	hostA, hostB := strings.ToLower(urlA.Hostname()), strings.ToLower(urlB.Hostname())
	if hostA == "" || hostB == "" {
		return false
	}
	if hostA == hostB {
		return true
	}
	domainA, domainErrA := publicsuffix.EffectiveTLDPlusOne(hostA)
	domainB, domainErrB := publicsuffix.EffectiveTLDPlusOne(hostB)
	if domainErrA != nil || domainErrB != nil {
		return false
	}
	if alias, ok := domainAliases[domainA]; ok {
		domainA = alias
	}
	if alias, ok := domainAliases[domainB]; ok {
		domainB = alias
	}
	return domainA == domainB
}

// ParseMercuryJSONTitle extracts the "title" field from a mercury/article.json
// document. It returns an empty string if the field is absent, empty, or the
// data is not valid JSON.
func ParseMercuryJSONTitle(data []byte) string {
	var info struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return ""
	}
	return strings.TrimSpace(info.Title)
}

// BestTitle returns the best available title for the snapshot in dir,
// trying sources in priority order: yt-dlp info.json, mercury/article.json,
// obelisk singlefile HTML, then wget and curl DOM HTML. Empty is returned
// when no source has a title.
func BestTitle(dir string) string {
	var infoTitle string
	if matches, _ := filepath.Glob(filepath.Join(dir, "*.info.json")); len(matches) > 0 {
		if data, err := os.ReadFile(matches[0]); err == nil {
			title, _, extractor := ParseInfoJSON(data)
			if title != "" {
				// A specialized extractor (youtube, etc.) has high-quality
				// metadata; the "generic" extractor does not, so demote it
				// to fallback priority (used only if no other source has a title).
				if extractor != "" && extractor != "generic" {
					return title
				}
				infoTitle = title
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "mercury", "article.json")); err == nil {
		if t := ParseMercuryJSONTitle(data); t != "" {
			return t
		}
	}
	if htmlContent, err := os.ReadFile(filepath.Join(dir, obelisk.OutputFile)); err == nil {
		if t := ParseTitle(htmlContent); t != "" {
			return t
		}
	}
	if htmlContent, err := os.ReadFile(filepath.Join(dir, obeliskproxy.OutputFile)); err == nil {
		if t := ParseTitle(htmlContent); t != "" {
			return t
		}
	}
	if htmlContent, err := os.ReadFile(filepath.Join(dir, wget.OutputFile)); err == nil {
		if t := ParseTitle(htmlContent); t != "" {
			return t
		}
	}
	if htmlContent, err := os.ReadFile(filepath.Join(dir, curl.OutputFile)); err == nil {
		if t := ParseTitle(htmlContent); t != "" {
			return t
		}
	}
	return infoTitle
}
