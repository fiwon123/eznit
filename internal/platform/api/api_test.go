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

// Every /v1 resource is mounted and guarded: a route that answered 404 would
// be a route the router never registered, and an unguarded one would be a
// route that skips the session check.
func TestTheRoutesAreMountedAndGuarded(t *testing.T) {
	router := newTestRouter(t)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"the file list", http.MethodGet, "/v1/files"},
		{"the caller's files alias", http.MethodGet, "/v1/files/me"},
		{"an upload", http.MethodPost, "/v1/files"},
		{"a download", http.MethodGet, "/v1/files/1f0c1a9e-0000-0000-0000-000000000000/content"},
		{"the template catalogue", http.MethodGet, "/v1/templates"},
		{"a login", http.MethodPost, "/v1/login"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.path, nil))

			if recorder.Code == http.StatusNotFound {
				t.Fatalf("%s %s = 404, want a registered route", tt.method, tt.path)
			}

			// /v1/login is public by design; everything else needs a
			// session, and no request here carries one.
			if tt.path != "/v1/login" && recorder.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without a token = %d, want %d", tt.method, tt.path, recorder.Code, http.StatusUnauthorized)
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
