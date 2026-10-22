package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The web UI is a single page application: the browser owns the routes, so
// an unknown path has to answer with index.html and not with a 404. The API
// keeps its own paths.
func TestWebHandler(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, filepath.Join(dist, "index.html"), "<html>the app</html>")
	writeFile(t, filepath.Join(dist, "assets", "app.js"), "console.log('the app');")

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("the api"))
	})
	handler := webHandler(dist, api)

	cases := []struct {
		name   string
		path   string
		status int
		body   string
	}{
		{"the root serves the app", "/", http.StatusOK, "the app"},
		{"an unknown path serves the app", "/templates/abc", http.StatusOK, "the app"},
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

// A path that tries to climb out of the dist folder must not serve a file
// from outside it. net/http answers those with a 400 of its own, so the
// assertion is about what the body holds and not about the status.
func TestWebHandlerDoesNotServeOutsideTheDistFolder(t *testing.T) {
	dist := t.TempDir()
	writeFile(t, filepath.Join(dist, "index.html"), "<html>the app</html>")

	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "OUTSIDE-SECRET")

	handler := webHandler(dist, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		req := httptest.NewRequest(http.MethodGet, target, nil)
		handler.ServeHTTP(rec, req)

		if strings.Contains(rec.Body.String(), "OUTSIDE-SECRET") {
			t.Errorf("%s: the handler served a file outside the dist folder", target)
		}
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

func writeFile(t *testing.T, name, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
