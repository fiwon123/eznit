package server

import (
	"context"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
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

func TestAPIPort(t *testing.T) {
	t.Setenv("API_PORT", "")
	if got := apiPort(); got != 4000 {
		t.Errorf("apiPort() = %d, want %d", got, 4000)
	}

	t.Setenv("API_PORT", "8080")
	if got := apiPort(); got != 8080 {
		t.Errorf("apiPort() = %d, want %d", got, 8080)
	}

	t.Setenv("API_PORT", "not-a-number")
	if got := apiPort(); got != 4000 {
		t.Errorf("apiPort() = %d, want %d", got, 4000)
	}

	t.Setenv("API_PORT", "0")
	if got := apiPort(); got != 4000 {
		t.Errorf("apiPort() = %d, want %d", got, 4000)
	}

	t.Setenv("API_PORT", "70000")
	if got := apiPort(); got != 4000 {
		t.Errorf("apiPort() = %d, want %d", got, 4000)
	}
}

func TestWebUIEnabled(t *testing.T) {
	t.Setenv("API_SERVE_WEB", "")
	if !webUIEnabled() {
		t.Error("webUIEnabled() = false, want true by default")
	}

	t.Setenv("API_SERVE_WEB", "false")
	if webUIEnabled() {
		t.Error("webUIEnabled() = true, want false")
	}

	t.Setenv("API_SERVE_WEB", "0")
	if webUIEnabled() {
		t.Error("webUIEnabled() = true, want false")
	}

	t.Setenv("API_SERVE_WEB", "true")
	if !webUIEnabled() {
		t.Error("webUIEnabled() = false, want true")
	}

	t.Setenv("API_SERVE_WEB", "1")
	if !webUIEnabled() {
		t.Error("webUIEnabled() = false, want true")
	}

	t.Setenv("API_SERVE_WEB", "not-a-bool")
	if !webUIEnabled() {
		t.Error("webUIEnabled() = false, want true when unparsable")
	}
}

// listenLocal binds a listener a test can close, on a port the kernel picks.
func listenLocal(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	return listener
}

// waitForAnswer retries until the server accepts a request, so a test never
// races the goroutine that calls Serve.
func waitForAnswer(t *testing.T, addr string) {
	t.Helper()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("the server never answered")
}

// A cancel stops the server and closes its port: a start that follows hard on
// a stop must not find a listener nobody owns.
func TestServeUntilStopsOnCancel(t *testing.T) {
	listener := listenLocal(t)
	addr := listener.Addr().String()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveUntil(ctx, srv, listener, 5*time.Second) }()

	waitForAnswer(t, addr)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveUntil() = %v, want nil after a clean drain", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil() did not return after the context was cancelled")
	}

	client := &http.Client{Timeout: 300 * time.Millisecond}
	if resp, err := client.Get("http://" + addr + "/"); err == nil {
		resp.Body.Close()
		t.Error("the port still answers after the shutdown")
	}
}

// A stop is a drain: a request already being handled finishes, and its
// response arrives, before serveUntil returns.
func TestServeUntilDrainsInFlightRequests(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("finished"))
	})}

	listener := listenLocal(t)
	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveUntil(ctx, srv, listener, 5*time.Second) }()

	type answer struct {
		code int
		body string
		err  error
	}
	answers := make(chan answer, 1)
	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + addr + "/upload")
		if err != nil {
			answers <- answer{err: err}
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		answers <- answer{code: resp.StatusCode, body: string(body), err: err}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the handler")
	}

	cancel()
	close(release)

	select {
	case got := <-answers:
		if got.err != nil {
			t.Fatalf("the in-flight request failed during the drain: %v", got.err)
		}
		if got.code != http.StatusOK || got.body != "finished" {
			t.Errorf("the in-flight request answered %d %q, want %d %q",
				got.code, got.body, http.StatusOK, "finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request never answered")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveUntil() = %v, want nil after the drain", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil() did not return after the drain")
	}
}

// A request that outlives the drain is cut off rather than holding the process
// open, and the caller learns the drain failed.
func TestServeUntilReportsADrainThatTimedOut(t *testing.T) {
	entered := make(chan struct{})
	stuck := make(chan struct{})
	defer close(stuck)

	srv := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-stuck
	})}

	listener := listenLocal(t)
	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveUntil(ctx, srv, listener, 100*time.Millisecond) }()

	go func() {
		client := &http.Client{Timeout: 10 * time.Second}
		if resp, err := client.Get("http://" + addr + "/stuck"); err == nil {
			resp.Body.Close()
		}
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the handler")
	}

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("serveUntil() = nil, want the drain timeout reported")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil() did not return after the drain timed out")
	}
}
