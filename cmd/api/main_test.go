package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// builtUI is a stand-in for a `make web_build` output, labelled so a test can
// tell which copy answered.
func builtUI(label string) fstest.MapFS {
	return fstest.MapFS{
		"index.html": {Data: []byte("<html>" + label + "</html>")},
	}
}

// The unbuilt state: web/dist exists for //go:embed, but holds only the
// committed placeholder.
func placeholderUI() fstest.MapFS {
	return fstest.MapFS{".gitkeep": {Data: nil}}
}

func apiHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("the api"))
	})
}

// A built web/dist wins over the embedded copy — unless it holds only the
// placeholder, in which case there is no UI there to serve.
func TestPickWebUI(t *testing.T) {
	missing := os.DirFS(filepath.Join(t.TempDir(), "not-built"))

	tests := []struct {
		name     string
		disk     fs.FS
		embedded fs.FS
		body     string // the UI that answers, or "" when there is none
	}{
		{"the disk copy wins when both are built", builtUI("the disk app"), builtUI("the embedded app"), "the disk app"},
		{"the disk copy serves when nothing is embedded", builtUI("the disk app"), placeholderUI(), "the disk app"},
		{"the embedded copy serves when the disk has only the placeholder", placeholderUI(), builtUI("the embedded app"), "the embedded app"},
		{"the embedded copy serves when the disk folder is missing", missing, builtUI("the embedded app"), "the embedded app"},
		{"nothing to serve when neither is built", placeholderUI(), placeholderUI(), ""},
		{"nothing to serve when nothing is there", missing, fstest.MapFS{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui, source := pickWebUI(tt.disk, tt.embedded, apiHandler(t))

			if tt.body == "" {
				if ui != nil {
					t.Errorf("pickWebUI() returned a handler with source %q, want none", source)
				}
				if source != "" {
					t.Errorf("source = %q, want it empty when there is no UI", source)
				}
				return
			}

			if ui == nil {
				t.Fatalf("pickWebUI() returned no handler, want the UI that serves %q", tt.body)
			}
			if source == "" {
				t.Error("source is empty, so the log could not say where the UI came from")
			}

			for _, target := range []string{"/", "/templates/abc"} {
				rec := httptest.NewRecorder()
				ui.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

				if rec.Code != http.StatusOK {
					t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusOK)
				}
				if !strings.Contains(rec.Body.String(), tt.body) {
					t.Errorf("%s: body = %q, want it to hold %q", target, rec.Body.String(), tt.body)
				}
			}
		})
	}
}

// The API keeps its own paths whichever copy of the UI is in front of it.
func TestPickWebUIKeepsTheAPIPaths(t *testing.T) {
	ui, _ := pickWebUI(placeholderUI(), builtUI("the embedded app"), apiHandler(t))
	if ui == nil {
		t.Fatal("pickWebUI() returned no handler")
	}

	rec := httptest.NewRecorder()
	ui.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/files/me", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d (the API's answer)", rec.Code, http.StatusTeapot)
	}
	if !strings.Contains(rec.Body.String(), "the api") {
		t.Errorf("body = %q, want the API's", rec.Body.String())
	}
}

// The dist folder is optional: without it the API answers on its own.
func TestWebDistFolder(t *testing.T) {
	t.Setenv("API_WEB_DIST", "")
	if got := webDistFolder(); got != "web/dist" {
		t.Errorf("webDistFolder() = %q, want %q", got, "web/dist")
	}

	t.Setenv("API_WEB_DIST", "/srv/eznit/web")
	if got := webDistFolder(); got != "/srv/eznit/web" {
		t.Errorf("webDistFolder() = %q, want %q", got, "/srv/eznit/web")
	}
}
