package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// builtUI is a two-file stand-in for a `make web_build` output.
func builtUI() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>the app</html>")},
		"assets/app.js": {Data: []byte("console.log('the app');")},
	}
}

func apiHandler(t *testing.T) http.Handler {
	t.Helper()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("the api"))
	})
}

// The web UI is a single page application: the browser owns the routes, so
// an unknown path has to answer with index.html and not with a 404. The API
// keeps its own paths.
func TestHandler(t *testing.T) {
	handler := Handler(builtUI(), apiHandler(t))

	cases := []struct {
		name   string
		path   string
		status int
		body   string
	}{
		{"the root serves the app", "/", http.StatusOK, "the app"},
		{"an unknown path serves the app", "/templates/abc", http.StatusOK, "the app"},
		{"a deep unknown path serves the app", "/files/1/edit", http.StatusOK, "the app"},
		{"an asset is served as itself", "/assets/app.js", http.StatusOK, "console.log"},
		{"the api keeps its own paths", "/v1/templates", http.StatusTeapot, "the api"},
		{"an unknown api path keeps the api's answer", "/v1/nope", http.StatusTeapot, "the api"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Errorf("%s: status = %d, want %d", tt.path, rec.Code, tt.status)
			}
			if !strings.Contains(rec.Body.String(), tt.body) {
				t.Errorf("%s: body = %q, want it to hold %q", tt.path, rec.Body.String(), tt.body)
			}
		})
	}
}

// The api handler must never be reached for a UI path, and the file server
// must never answer an api path.
func TestHandlerKeepsTheTwoSidesApart(t *testing.T) {
	apiCalled := 0
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled++
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Handler(builtUI(), api)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/templates", nil))
	if apiCalled != 0 {
		t.Errorf("the api handled a UI path: called %d times", apiCalled)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/files/me", nil))
	if apiCalled != 1 {
		t.Errorf("the api was not handed the API path: called %d times, want 1", apiCalled)
	}
}

// A path that tries to climb out of the UI folder must not serve a file from
// outside it. net/http answers some of those with a 400 of its own, so the
// assertion is about what the body holds, not about the status.
func TestHandlerDoesNotServeOutsideTheRoot(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, filepath.Join(dist, "index.html"), "<html>the app</html>")

	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "OUTSIDE-SECRET")

	handler := Handler(os.DirFS(dist), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the api handler was called for %s", r.URL.Path)
	}))

	targets := []string{
		"/../secret.txt",
		"/../../etc/passwd",
		"/assets/../../secret.txt",
		"/%2e%2e/secret.txt",
	}

	for _, target := range targets {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

		if strings.Contains(rec.Body.String(), "OUTSIDE-SECRET") {
			t.Errorf("%s: the handler served a file outside the UI folder", target)
		}
	}
}

// The test is index.html and not the folder: an unbuilt checkout has a
// web/dist folder holding nothing but the committed placeholder.
func TestHasIndex(t *testing.T) {
	tests := []struct {
		name string
		fsys fstest.MapFS
		want bool
	}{
		{"an empty folder has no UI", fstest.MapFS{}, false},
		{"the placeholder is not a UI", fstest.MapFS{".gitkeep": {Data: nil}}, false},
		{"assets alone are not a UI", fstest.MapFS{"assets/app.js": {Data: []byte("x")}}, false},
		{"index.html is a UI", builtUI(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasIndex(tt.fsys); got != tt.want {
				t.Errorf("HasIndex(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}

	if HasIndex(nil) {
		t.Error("HasIndex(nil) = true, want false")
	}
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
