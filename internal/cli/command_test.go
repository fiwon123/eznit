package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// A template command reference is an id and a path, separated by `=`.
func TestParseCommandSpec(t *testing.T) {
	id := uuid.New()

	tests := []struct {
		name     string
		input    string
		wantID   uuid.UUID
		wantPath string
		wantOK   bool
	}{
		{"a reference", id.String() + "=scripts/fmt.sh", id, "scripts/fmt.sh", true},
		{"spaces around it", " " + id.String() + " = bin/tool ", id, "bin/tool", true},
		{"no separator", id.String(), uuid.Nil, "", false},
		{"not an id", "fmt=scripts/fmt.sh", uuid.Nil, "", false},
		{"no path", id.String() + "=", uuid.Nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, gotPath, ok := parseCommandSpec(tt.input)

			if ok != tt.wantOK {
				t.Fatalf("expected ok %t, got %t", tt.wantOK, ok)
			}

			if gotID != tt.wantID || gotPath != tt.wantPath {
				t.Errorf("expected (%s, %q), got (%s, %q)", tt.wantID, tt.wantPath, gotID, gotPath)
			}
		})
	}
}

// A command is created with the body the API expects: the content inline for
// text, base64 for binary.
func TestSendCommandRequestSendsTheBody(t *testing.T) {
	var gotMethod string
	var gotBody commands.CommandRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status_code":201,"data":{"id":"` + uuid.New().String() + `","name":"fmt","kind":"text","size":3,"content":"fmt","is_executable":true}}`))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)
	executable := true

	data, ok := sendCommandRequest(http.MethodPost, server.URL+"/v1/commands", commands.CommandRequest{
		Name:         "fmt",
		Content:      "fmt",
		IsExecutable: &executable,
	}, "the-token", globals)
	if !ok {
		t.Fatal("expected the command to be stored")
	}

	if gotMethod != http.MethodPost {
		t.Errorf("expected method %s, got %s", http.MethodPost, gotMethod)
	}

	if gotBody.Name != "fmt" || gotBody.Content != "fmt" {
		t.Errorf("expected the command body, got %+v", gotBody)
	}

	if data.Name != "fmt" {
		t.Errorf("expected the created command, got %+v", data)
	}
}

// A command is found by its name, not only by its id: a name is what a user
// remembers.
func TestResolveCommandFindsByName(t *testing.T) {
	id := uuid.New()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/commands" {
			t.Errorf("expected the listing, got %s", r.URL.Path)
		}

		w.Write([]byte(`{"status_code":200,"data":{"data":[{"id":"` + id.String() + `","name":"fmt","kind":"text","is_executable":true}],"total":1}}`))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	data, ok := resolveCommand(server.URL, "the-token", "fmt", globals)
	if !ok {
		t.Fatal("expected the command to be found")
	}

	if data.ID != id {
		t.Errorf("expected the command %s, got %s", id, data.ID)
	}
}
