package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
)

// The download route is behind the session guard, so the CLI has to send the
// stored token with the request. Without it the server answers 401 and no
// bytes are served.
func TestSendDownloadRequestSendsTheSessionToken(t *testing.T) {
	var gotAuthorization string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")

		w.Header().Set("Content-Disposition", "attachment; filename=secret.txt")
		w.Write([]byte("the bytes"))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	dest := t.TempDir()
	if err := sendDownloadRequest(server.URL, "the-id", dest, "the-token", globals); err != nil {
		t.Fatalf("expected the download to succeed, got error %v", err)
	}

	if gotAuthorization != "Bearer the-token" {
		t.Errorf("expected the request to carry the token, got %q", gotAuthorization)
	}

	content, err := os.ReadFile(filepath.Join(dest, "secret.txt"))
	if err != nil {
		t.Fatalf("expected the file to be written: %v", err)
	}

	if string(content) != "the bytes" {
		t.Errorf("expected the file content, got %q", string(content))
	}
}

// A rejected request must not leave a file behind.
func TestSendDownloadRequestWritesNothingWhenTheServerRefuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	dest := t.TempDir()
	if err := sendDownloadRequest(server.URL, "the-id", dest, "the-token", globals); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("failed to read the destination folder: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected no file to be written, found %d", len(entries))
	}
}
