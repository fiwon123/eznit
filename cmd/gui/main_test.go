package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/fiwon123/eznit/internal/platform/webui"
	"github.com/fiwon123/eznit/pkg/logger"
)

func testLogger(t *testing.T) *logger.Config {
	t.Helper()

	log, err := logger.NewJson(t.TempDir(), false)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { log.Sync() })

	return log
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

	return recorder
}

// A GUI given an API URL is a launcher: /v1 goes to that API, and the database
// configuration is not consulted.
func TestWhatAnswersV1ProxiesToTheConfiguredAPI(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/healthcheck" {
			t.Errorf("the proxy asked for %q, want /v1/healthcheck", r.URL.Path)
		}
		_, _ = w.Write([]byte("the api at the other end"))
	}))
	defer target.Close()

	t.Setenv("GUI_API_URL", target.URL)
	t.Setenv("DB_NAME", "")

	handler, source := whatAnswersV1(testLogger(t))

	if !strings.Contains(source, target.URL) {
		t.Errorf("whatAnswersV1() source = %q, want it to name %s", source, target.URL)
	}

	recorder := get(t, handler, "/v1/healthcheck")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/healthcheck = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); body != "the api at the other end" {
		t.Errorf("GET /v1/healthcheck body = %q, want the proxy's answer", body)
	}
}

// With no API and no database, the UI opens and every /v1 call says what to
// set. This is the state a fresh GUI binary is in, and a 404 from a static
// file server was the bug.
func TestWhatAnswersV1ExplainsWhatToConfigure(t *testing.T) {
	t.Setenv("GUI_API_URL", "")
	t.Setenv("DB_NAME", "")

	handler, source := whatAnswersV1(testLogger(t))

	recorder := get(t, handler, "/v1/files/me")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /v1/files/me = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	body := recorder.Body.String()
	for _, want := range []string{"GUI_API_URL", "DB_"} {
		if !strings.Contains(body, want) {
			t.Errorf("the message does not mention %q: %s", want, body)
		}
	}

	if !strings.Contains(source, "no API configured") {
		t.Errorf("whatAnswersV1() source = %q, want it to say no API is configured", source)
	}
}

func TestWhatAnswersV1RefusesAnUnusableAPIURL(t *testing.T) {
	t.Setenv("GUI_API_URL", "localhost:4000") // no scheme
	t.Setenv("DB_NAME", "")

	handler, _ := whatAnswersV1(testLogger(t))

	recorder := get(t, handler, "/v1/healthcheck")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /v1/healthcheck = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	if body := recorder.Body.String(); !strings.Contains(body, "GUI_API_URL") {
		t.Errorf("the message does not name GUI_API_URL: %s", body)
	}
}

// A configured database that is not there is a failure the window can show
// and the person can fix, not a panic and not a 404.
func TestWhatAnswersV1ExplainsAnUnreachableDatabase(t *testing.T) {
	t.Setenv("GUI_API_URL", "")
	t.Setenv("DB_NAME", "eznit")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "1")
	t.Setenv("DB_USER", "nobody")
	t.Setenv("DB_PWD", "not-a-real-password")
	t.Setenv("DB_PASSWORD_FILE", "")

	handler, source := whatAnswersV1(testLogger(t))

	recorder := get(t, handler, "/v1/healthcheck")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /v1/healthcheck = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	if body := recorder.Body.String(); !strings.Contains(body, "cannot reach") {
		t.Errorf("the message does not say the database is unreachable: %s", body)
	}

	if !strings.Contains(source, "unreachable") {
		t.Errorf("whatAnswersV1() source = %q, want it to say the database is unreachable", source)
	}
}

// The proxy's own failure path: a target that is gone answers 503 with the
// address, not a bare connection error.
func TestProxyAPIReportsAnUnreachableAPI(t *testing.T) {
	gone := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	address := gone.URL
	gone.Close()

	proxy, err := proxyAPI(address, testLogger(t))
	if err != nil {
		t.Fatalf("proxyAPI() error = %v", err)
	}

	recorder := get(t, proxy, "/v1/healthcheck")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /v1/healthcheck = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}

	if body := recorder.Body.String(); !strings.Contains(body, address) {
		t.Errorf("the message does not name the API address %s: %s", address, body)
	}
}

// The UI and the API share one origin: the page is still served, the API
// answers under /v1, and an unknown page is routed by index.html.
func TestTheUIAndTheAPIShareOneOrigin(t *testing.T) {
	ui := fstest.MapFS{
		"index.html": {Data: []byte("<html>the ui</html>")},
	}

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("the api"))
	})
	handler := webui.Handler(ui, api)

	if body := get(t, handler, "/").Body.String(); !strings.Contains(body, "the ui") {
		t.Errorf("GET / body = %q, want the UI", body)
	}

	if body := get(t, handler, "/files/abc").Body.String(); !strings.Contains(body, "the ui") {
		t.Errorf("GET /files/abc body = %q, want the SPA's index.html", body)
	}

	if body := get(t, handler, "/v1/healthcheck").Body.String(); body != "the api" {
		t.Errorf("GET /v1/healthcheck body = %q, want the API", body)
	}
}
