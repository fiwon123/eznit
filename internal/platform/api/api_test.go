package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
)

// The router is built with a nil database: every request below is answered
// before a query would run, so this covers the route table and the session
// guard without PostgreSQL.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()

	log, err := logger.NewJson(t.TempDir(), false)
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	t.Cleanup(func() { log.Sync() })

	return NewRouter(Config{Logger: log})
}

func TestTheHealthCheckAnswersWithoutADatabase(t *testing.T) {
	recorder := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/healthcheck", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/healthcheck = %d, want %d", recorder.Code, http.StatusOK)
	}

	if body := recorder.Body.String(); !strings.Contains(body, "available") {
		t.Errorf("GET /v1/healthcheck body = %q, want it to say available", body)
	}
}

// Every route that answers the caller's own rows is mounted and guarded: a
// route that answered 404 would be a route the router never registered, and a
// route that answered anything but 401 without a token would be a route that
// skips the session check.
func TestTheGuardedRoutesRejectAVisitor(t *testing.T) {
	router := newTestRouter(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"the file list", http.MethodGet, "/v1/files"},
		{"the caller's files alias", http.MethodGet, "/v1/files/me"},
		{"an upload", http.MethodPost, "/v1/files"},
		{"a file record", http.MethodGet, "/v1/files/1f0c1a9e-0000-0000-0000-000000000000"},
		{"a publish", http.MethodPost, "/v1/templates"},
		{"the caller's own templates", http.MethodGet, "/v1/templates/mine"},
		{"the command library", http.MethodGet, "/v1/commands"},
		{"the user list", http.MethodGet, "/v1/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))

			if recorder.Code == http.StatusNotFound {
				t.Fatalf("%s %s = 404, want a registered route", tt.method, tt.path)
			}

			if recorder.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without a token = %d, want %d", tt.method, tt.path, recorder.Code, http.StatusUnauthorized)
			}
		})
	}
}

// The public reads are mounted and are not guarded: without a token they reach
// their handler instead of the session check.
//
// The database here is nil, so a request that reached a repository would panic.
// The rows below name a value the handler refuses before it queries — a 400
// from the handler is what proves the guard let the request through. The list
// that does reach the database (the shared files) is covered by the files
// handler's own test, which runs against a fake repository.
func TestThePublicReadsAreNotGuarded(t *testing.T) {
	router := newTestRouter(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"the template catalogue", http.MethodGet, "/v1/templates?kind=bogus"},
		{"a template detail", http.MethodGet, "/v1/templates/not-an-id"},
		{"a download", http.MethodGet, "/v1/files/not-an-id/content"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))

			if recorder.Code == http.StatusNotFound {
				t.Fatalf("%s %s = 404, want a registered route", tt.method, tt.path)
			}

			if recorder.Code == http.StatusUnauthorized {
				t.Errorf("%s %s without a token = 401, want the public read to answer", tt.method, tt.path)
			}

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("%s %s = %d, want %d", tt.method, tt.path, recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestAnUnknownAPIPathIsNotFound(t *testing.T) {
	recorder := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/nope", nil))

	if recorder.Code != http.StatusNotFound {
		t.Errorf("GET /v1/nope = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
