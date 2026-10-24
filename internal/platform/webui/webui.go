// Package webui serves the built web UI in front of an API.
//
// The UI is a single page application: its routes live in the browser, so a
// path like /templates/<id> has to answer with index.html and not with a 404.
// The API keeps its own paths, so anything under /v1 goes to it untouched —
// including the 404 it gives for an unknown API path — and so does /docs, the
// reference the API serves.
//
// The handler works on an fs.FS instead of a directory, because the web UI
// reaches the process two ways: from web/dist on disk while developing, and
// from the copy embedded at build time (package web) in a released binary.
// Both are the same file system to this package.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// HasIndex reports whether fsys holds a built UI.
//
// The test is index.html, not the folder: a checkout that never ran
// `make web_build` still has web/dist — it holds the committed placeholder —
// and serving that would answer every page with an empty body.
func HasIndex(fsys fs.FS) bool {
	if fsys == nil {
		return false
	}

	info, err := fs.Stat(fsys, "index.html")
	return err == nil && !info.IsDir()
}

// apiPath reports whether the API owns a path.
//
// Everything under /v1 is the API's. So is /docs, where the API serves its own
// OpenAPI reference (internal/platform/openapi) — the UI has no page there, and
// letting the SPA fallback answer it would hide the reference behind the app.
func apiPath(p string) bool {
	return strings.HasPrefix(p, "/v1/") || p == "/docs" || strings.HasPrefix(p, "/docs/")
}

// Handler serves fsys in front of api.
//
// A request under /v1 is the API's, and so is the documentation the API serves
// at /docs; every other request is the UI's. A path that names a file in fsys
// gets that file, and anything else gets index.html for the browser to route.
func Handler(fsys fs.FS, api http.Handler) http.Handler {
	files := http.FileServerFS(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiPath(r.URL.Path) {
			api.ServeHTTP(w, r)
			return
		}

		// path.Clean collapses "..", and the result is relative to the root
		// of fsys, so a crafted path cannot name a file outside it.
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name != "" {
			if info, err := fs.Stat(fsys, name); err == nil && !info.IsDir() {
				files.ServeHTTP(w, r)
				return
			}
		}

		http.ServeFileFS(w, r, fsys, "index.html")
	})
}
