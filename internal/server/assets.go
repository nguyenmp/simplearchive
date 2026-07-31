package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/static
var staticFS embed.FS

// staticFiles returns the embedded assets/static subtree as an fs.FS suitable
// for http.FileServerFS (Go 1.22+). It strips the "assets/static/" prefix.
func staticFiles() fs.FS {
	sub, err := fs.Sub(staticFS, "assets/static")
	if err != nil {
		// assets/static is compile-time embedded; Sub can only fail if the path
		// is wrong, which is a programmer error. Panic to surface it loudly.
		panic("server: embed assets/static: " + err.Error())
	}
	return sub
}

// staticHandler returns an http.Handler that serves the embedded static assets
// (tailwind.css) under /static/. It sets long-lived cache headers
// and nosniff, and strips the /static/ prefix before delegating to
// http.FileServerFS.
func staticHandler() http.Handler {
	fileServer := http.FileServerFS(staticFiles())
	return http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fileServer.ServeHTTP(w, r)
	}))
}
