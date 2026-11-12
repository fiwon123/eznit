package apiclient

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/templates"
)

// newTemplateServer answers the two read routes with a fixed body and records
// the path it was asked for.
func newTemplateServer(t *testing.T, wantPath string, body any) *Client {
	t.Helper()

	return testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != wantPath {
			t.Errorf("path = %q, want %q", r.URL.RequestURI(), wantPath)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer the-token" {
			t.Errorf("Authorization = %q, want the session: the catalogue is public, but the token is what puts the caller's own templates in it", got)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Fatal(err)
		}
	})
}

// The catalogue is one request, and a nil list is an empty list rather than an
// error: a first install has no templates.
func TestListTemplates(t *testing.T) {
	t.Run("catalogue", func(t *testing.T) {
		client := newTemplateServer(t, "/v1/templates", map[string]any{
			"data": map[string]any{
				"data":  []map[string]any{{"id": "6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11", "name": "starter", "kind": "generic", "version": 2, "file_count": 3}},
				"total": 1,
			},
		})

		items, err := client.ListTemplates(false, "")
		if err != nil {
			t.Fatal(err)
		}

		if len(items) != 1 || items[0].Name != "starter" || items[0].FileCount != 3 {
			t.Fatalf("items = %+v, want the one template", items)
		}
	})

	t.Run("empty", func(t *testing.T) {
		client := newTemplateServer(t, "/v1/templates", map[string]any{"data": map[string]any{}})

		items, err := client.ListTemplates(false, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 {
			t.Errorf("items = %v, want none", items)
		}
	})

	t.Run("mine", func(t *testing.T) {
		client := newTemplateServer(t, "/v1/templates/mine", map[string]any{"data": map[string]any{}})

		if _, err := client.ListTemplates(true, ""); err != nil {
			t.Fatal(err)
		}
	})

	// The kind filter is the server's when the catalogue route answers, and it
	// is not offered by "mine" — sending it there would be silently ignored.
	t.Run("kind", func(t *testing.T) {
		client := newTemplateServer(t, "/v1/templates?kind=opencode", map[string]any{"data": map[string]any{}})

		if _, err := client.ListTemplates(false, templates.KindOpencode); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("kind with mine is filtered here", func(t *testing.T) {
		client := newTemplateServer(t, "/v1/templates/mine", map[string]any{"data": map[string]any{}})

		if _, err := client.ListTemplates(true, templates.KindOpencode); err != nil {
			t.Fatal(err)
		}
	})
}

// The detail is the whole template: files, variables and commands.
func TestGetTemplate(t *testing.T) {
	body := map[string]any{
		"data": map[string]any{
			"id":          "6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11",
			"name":        "starter",
			"kind":        "generic",
			"version":     2,
			"files":       []map[string]any{{"path": "AGENTS.md", "kind": "text", "content": "# hi", "size": 4}},
			"variables":   []map[string]any{{"key": "PROJECT", "prompt": "Project", "required": true}},
			"commands":    []map[string]any{{"target_path": "scripts/run.sh", "name": "run", "kind": "text", "content": "echo hi"}},
			"file_count":  1,
			"description": "a starter",
		},
	}

	client := newTemplateServer(t, "/v1/templates/6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11", body)

	detail, err := client.GetTemplate("6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11")
	if err != nil {
		t.Fatal(err)
	}

	if detail.Name != "starter" || len(detail.Files) != 1 || detail.Files[0].Content != "# hi" {
		t.Fatalf("detail = %+v, want the one file with its content inline", detail)
	}
	if len(detail.Variables) != 1 || !detail.Variables[0].Required {
		t.Errorf("variables = %+v, want the required one", detail.Variables)
	}
	if len(detail.Commands) != 1 || detail.Commands[0].TargetPath != "scripts/run.sh" {
		t.Errorf("commands = %+v, want the pulled-in one", detail.Commands)
	}
}

// A missing template is the API's own error, not an empty detail.
func TestGetTemplateMissing(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"template not found"}`))
	})

	_, err := client.GetTemplate("nope")
	if err == nil {
		t.Fatal("GetTemplate() = nil, want an error")
	}

	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusNotFound || apiErr.Message != "template not found" {
		t.Errorf("error = %v, want the host's 404", err)
	}
}

// FilterTemplates is what a "mine plus kind" request needs, and it must not
// invent or drop anything else.
func TestFilterTemplates(t *testing.T) {
	items := []templates.TemplateData{
		{Name: "a", Kind: templates.KindOpencode},
		{Name: "b", Kind: templates.KindGeneric},
		{Name: "c", Kind: templates.KindOpencode},
	}

	if got := FilterTemplates(items, ""); len(got) != 3 {
		t.Errorf("no kind kept %d, want all 3", len(got))
	}

	kept := FilterTemplates(items, templates.KindOpencode)
	if len(kept) != 2 || kept[0].Name != "a" || kept[1].Name != "c" {
		t.Errorf("kept = %+v, want the two opencode templates in order", kept)
	}

	if got := FilterTemplates(items, templates.KindFish); len(got) != 0 {
		t.Errorf("an unknown kind kept %d, want none", len(got))
	}
}
