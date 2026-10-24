package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
)

// The list answers the caller's own files, so the route is behind the session
// guard: the CLI has to send the token with the request, whether it asks for
// the default list or names `/me` explicitly.
func TestSendListRequestSendsTheSessionToken(t *testing.T) {
	var gotAuthorization string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		w.Write([]byte(`{"data":[],"total":0}`))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	sendListRequest(server.URL, false, "the-token", globals)

	if gotAuthorization != "Bearer the-token" {
		t.Errorf("expected the request to carry the token, got %q", gotAuthorization)
	}

	if gotPath != "/v1/files" {
		t.Errorf("expected the list route, got %q", gotPath)
	}
}
