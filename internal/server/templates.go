package server

import (
	"embed"
	"html/template"
	"io"
	"sync"
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
	if t, ok := r.sets[name]; ok {
		return t, nil
	}
	t := template.New("").Funcs(template.FuncMap{
		"formatTimestamp": formatTimestamp,
		"snapshotPath":    snapshotPath,
	})
	if _, err := t.ParseFS(
		templateFS,
		"templates/layout.html",
		"templates/"+name+".html",
	); err != nil {
		return nil, err
	}
	r.sets[name] = t
	return t, nil
}

// render executes the named page's template set against data, writing the full
// HTML document to w.
func (r *renderer) render(w io.Writer, page string, data any) error {
	t, err := r.page(page)
	if err != nil {
		return err
	}
	return t.ExecuteTemplate(w, "layout.html", data)
}
