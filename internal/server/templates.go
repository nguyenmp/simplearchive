package server

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"sync"
	"time"

	"github.com/nguyenmp/simplearchive/internal/snapshot"
)

//go:embed templates
var templateFS embed.FS

// renderer lazily parses and caches template sets, one per page. Each set pairs
// the shared layout.html with a page template that overrides the "content"
// block. Pages are parsed on first use so adding a new page template does not
// require touching this file.
type renderer struct {
	mu   sync.Mutex
	sets map[string]*template.Template
}

func newRenderer() *renderer {
	return &renderer{sets: make(map[string]*template.Template)}
}

func (r *renderer) page(name string) (*template.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tmpl, ok := r.sets[name]; ok {
		return tmpl, nil
	}
	tmpl := template.New("").Funcs(template.FuncMap{
		"formatTimestamp": formatTimestamp,
		"snapshotPath":    snapshotPath,
		"statusClass":     statusClass,
		"faviconPath":     faviconPath,
		"timeAgo":         timeAgo,
		"humanSize":       humanSize,
		"runDuration":     runDuration,
	})
	if _, err := tmpl.ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/"+name+".html",
	); err != nil {
		return nil, err
	}
	r.sets[name] = tmpl
	return tmpl, nil
}

// render executes the named page's template set against data, writing the full
// HTML document to w.
func (r *renderer) render(w io.Writer, page string, data any) error {
	tmpl, err := r.page(page)
	if err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "layout.html", data)
}

// faviconPath returns the URL path to a snapshot's favicon in the archive.
func faviconPath(timestamp int64) string {
	return "/archive/" + snapshot.Format(timestamp) + "/favicon.ico"
}

// timeAgo renders a human-readable relative time string, e.g. "3 minutes ago".
func timeAgo(timestamp int64) string {
	duration := time.Since(time.UnixMicro(timestamp))
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", minutes)
	case duration < 24*time.Hour:
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		day := int(duration.Hours()) / 24
		if day == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", day)
	}
}

// humanSize renders a byte count as a human-readable string (B, KB, MB).
func humanSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	}
}

// runDuration renders an extractor run's wall-clock duration in whole seconds,
// e.g. "14s". It returns "" when the run has not finished (missing start or
// finish timestamp) so pending/running runs show nothing.
func runDuration(startedAt, finishedAt int64) string {
	if startedAt == 0 || finishedAt == 0 || finishedAt < startedAt {
		return ""
	}
	duration := time.Duration(finishedAt-startedAt) * time.Microsecond
	return fmt.Sprintf("%ds", int64(duration.Round(time.Second).Seconds()))
}

// statusClass returns the Tailwind badge classes for an extractor run status.
func statusClass(status string) string {
	switch status {
	case "succeeded":
		return "bg-green-100 text-green-800"
	case "failed":
		return "bg-red-100 text-red-800"
	case "skipped":
		return "bg-gray-200 text-gray-600"
	case "running", "pending":
		return "bg-amber-100 text-amber-800"
	default:
		return "bg-gray-200 text-gray-600"
	}
}
