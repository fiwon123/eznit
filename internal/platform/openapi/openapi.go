// Package openapi serves the API's OpenAPI document and a Swagger UI over it.
//
// The document is committed next to this file (openapi.yaml) and embedded in
// the binary, so a release serves its own contract with no extra files and no
// network access. It describes the routes internal/platform/api mounts, and
// the drift gate in that package fails when the two disagree.
package openapi

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-chi/chi/v5"
	swaggerFiles "github.com/swaggo/files/v2"
	"gopkg.in/yaml.v3"
)

const (
	// SpecPath is where the document is served as YAML.
	SpecPath = "/v1/openapi.yaml"
	// SpecJSONPath is the same document as JSON. It is what the UI loads, and
	// what most tooling prefers.
	SpecJSONPath = "/v1/openapi.json"
	// DocsPath is the Swagger UI.
	DocsPath = "/docs/"
)

//go:embed openapi.yaml
var specYAML []byte

//go:embed index.html
var indexHTML []byte

var (
	jsonOnce sync.Once
	specJSON []byte
	jsonErr  error
)

// YAML returns the embedded document, unchanged.
func YAML() []byte {
	return specYAML
}

// JSON returns the embedded document converted to JSON.
//
// The conversion happens once. A document that cannot be parsed is a build
// mistake, not a request error: the handler answers 500 rather than panicking
// at startup, and the package's tests fail before such a build ships.
func JSON() ([]byte, error) {
	jsonOnce.Do(func() {
		var document map[string]any
		if err := yaml.Unmarshal(specYAML, &document); err != nil {
			jsonErr = err
			return
		}

		specJSON, jsonErr = json.Marshal(document)
	})

	return specJSON, jsonErr
}

// Mount registers the documentation routes on r.
//
// None of them needs a session: a client author has to be able to read the
// contract before they have a token. They expose the route table and the
// schemas — never a row, a file or a token.
func Mount(r chi.Router) {
	r.Get(SpecPath, serveSpecYAML)
	r.Get(SpecJSONPath, serveSpecJSON)

	// The UI is a directory of files, and its relative links assume the
	// trailing slash. Send /docs there, then serve the UI's own page from the
	// embedded assets.
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, DocsPath, http.StatusMovedPermanently)
	})
	r.Get(DocsPath, serveDocs)

	assets := http.StripPrefix("/docs/", http.FileServerFS(swaggerFiles.FS))
	r.Get("/docs/*", assets.ServeHTTP)
	r.Head("/docs/*", assets.ServeHTTP)
}

func serveSpecYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(specYAML)
}

func serveSpecJSON(w http.ResponseWriter, _ *http.Request) {
	document, err := JSON()
	if err != nil {
		http.Error(w, "the embedded OpenAPI document is not valid YAML", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func serveDocs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(indexHTML)
}
