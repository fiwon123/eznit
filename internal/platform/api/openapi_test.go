package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/commands"
	"github.com/fiwon123/eznit/internal/domain/files"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/domain/users"
	"github.com/fiwon123/eznit/internal/platform/openapi"
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// The documentation routes are not part of the API contract: they serve the
// contract itself. Everything else the router answers has to be in the
// document, and everything in the document has to be answered by the router.
var notInTheContract = map[string]bool{
	"GET /v1/openapi.yaml": true,
	"GET /v1/openapi.json": true,
	"GET /docs":            true,
	"GET /docs/*":          true,
	"HEAD /docs/*":         true,
}

// TestTheRouterAndTheDocumentAgree is the drift gate. A route added without a
// line in openapi.yaml, or a line left behind by a removed route, fails here
// rather than in a client author's hands.
func TestTheRouterAndTheDocumentAgree(t *testing.T) {
	router, ok := newTestRouter(t).(*chi.Mux)
	if !ok {
		t.Fatal("the router is not a *chi.Mux")
	}

	served := map[string]bool{}
	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		served[method+" "+normalizePath(route)] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk the router: %v", err)
	}

	for operation := range notInTheContract {
		if !served[operation] {
			t.Errorf("the documentation route %s is no longer mounted", operation)
		}
	}

	documented := documentedOperations(t)

	for operation := range served {
		if notInTheContract[operation] {
			continue
		}

		if !documented[operation] {
			t.Errorf("the router serves %s but the document does not describe it", operation)
		}
	}

	for operation := range documented {
		if !served[operation] {
			t.Errorf("the document describes %s but the router does not serve it", operation)
		}
	}
}

// The documentation is public on purpose: a client author reads the contract
// before they have a token. These routes are mounted outside every session
// guard, and this is what proves it — the gate above only proves they exist.
func TestTheDocumentationAnswersWithoutASession(t *testing.T) {
	router := newTestRouter(t)

	tests := []struct {
		name   string
		path   string
		status int
	}{
		{"the document", openapi.SpecPath, http.StatusOK},
		{"the document as JSON", openapi.SpecJSONPath, http.StatusOK},
		{"the UI page", openapi.DocsPath, http.StatusOK},
		{"the bare UI path", "/docs", http.StatusMovedPermanently},
		{"a UI asset", "/docs/swagger-ui.css", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if recorder.Code != tt.status {
				t.Errorf("GET %s = %d, want %d", tt.path, recorder.Code, tt.status)
			}
		})
	}
}

func documentedOperations(t *testing.T) map[string]bool {
	t.Helper()

	var document struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}

	if err := yaml.Unmarshal(openapi.YAML(), &document); err != nil {
		t.Fatalf("parse the embedded document: %v", err)
	}

	operations := map[string]bool{}
	for path, item := range document.Paths {
		for method := range item {
			switch method {
			case "get", "put", "post", "delete", "patch", "head", "options", "trace":
				operations[strings.ToUpper(method)+" "+normalizePath(path)] = true
			}
		}
	}

	return operations
}

// normalizePath removes the trailing slash chi keeps on a subrouter's root, so
// `/v1/files/` and `/v1/files` are the same operation.
func normalizePath(path string) string {
	if path == "/" {
		return path
	}

	return strings.TrimSuffix(path, "/")
}

// A schema that drifts from its Go type is worse than a missing one: the
// document renders and the client sends the wrong field. The schemas that
// mirror a DTO are checked against its json tags.
func TestTheSchemasMatchTheirDTOs(t *testing.T) {
	document := parseSchemas(t)

	cases := []struct {
		schema string
		value  any
	}{
		{"LoginRequest", users.LoginRequest{}},
		{"SignupRequest", users.SignupRequest{}},
		{"UpdateUserRequest", users.UpdateRequest{}},
		{"UserData", users.UserData{}},
		{"LoginResponse", users.LoginResponse{}},
		{"FileData", files.FileData{}},
		{"FileList", files.ListResponse{}},
		{"TemplateData", templates.TemplateData{}},
		{"TemplateFileData", templates.TemplateFileData{}},
		{"TemplateVariableData", templates.TemplateVariableData{}},
		{"TemplateCommandData", templates.TemplateCommandData{}},
		{"TemplateDetail", templates.TemplateDetail{}},
		{"TemplateList", templates.ListResponse{}},
		{"PublishFile", templates.PublishFile{}},
		{"PublishCommand", templates.PublishCommand{}},
		{"PublishVariable", templates.PublishVariable{}},
		{"PublishRequest", templates.PublishRequest{}},
		{"CommandData", commands.CommandData{}},
		{"CommandRequest", commands.CommandRequest{}},
		{"CommandList", commands.ListResponse{}},
	}

	for _, tt := range cases {
		t.Run(tt.schema, func(t *testing.T) {
			want := jsonNames(reflect.TypeOf(tt.value))
			got := schemaProperties(t, document, tt.schema)

			for name := range want {
				if !got[name] {
					t.Errorf("the DTO has json field %q but the schema does not", name)
				}
			}

			for name := range got {
				if !want[name] {
					t.Errorf("the schema has property %q but the DTO has no such json field", name)
				}
			}
		})
	}
}

// files.File carries db tags and no json tags, so GET /v1/files/{id} sends the
// Go field names. The schema has to say so, not a nicer shape.
func TestTheFileRecordSchemaUsesTheStorageFieldNames(t *testing.T) {
	document := parseSchemas(t)
	got := schemaProperties(t, document, "FileRecord")

	typ := reflect.TypeOf(files.File{})
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !got[name] {
			t.Errorf("the schema does not name the field %q", name)
		}
		delete(got, name)
	}

	for name := range got {
		t.Errorf("the schema has property %q but files.File has no such field", name)
	}
}

type schemaNode struct {
	Properties map[string]schemaNode `yaml:"properties"`
	AllOf      []schemaNode          `yaml:"allOf"`
	Ref        string                `yaml:"$ref"`
}

type schemaDocument struct {
	Components struct {
		Schemas map[string]schemaNode `yaml:"schemas"`
	} `yaml:"components"`
}

func parseSchemas(t *testing.T) schemaDocument {
	t.Helper()

	var document schemaDocument
	if err := yaml.Unmarshal(openapi.YAML(), &document); err != nil {
		t.Fatalf("parse the embedded document: %v", err)
	}

	return document
}

// schemaProperties collects the property names of a schema, following `$ref`
// and merging `allOf`.
func schemaProperties(t *testing.T, document schemaDocument, name string) map[string]bool {
	t.Helper()

	node, ok := document.Components.Schemas[name]
	if !ok {
		t.Fatalf("the document has no schema named %q", name)
	}

	return propertiesOf(t, document, node, map[string]bool{})
}

func propertiesOf(t *testing.T, document schemaDocument, node schemaNode, seen map[string]bool) map[string]bool {
	t.Helper()

	properties := map[string]bool{}

	if node.Ref != "" {
		name := strings.TrimPrefix(node.Ref, "#/components/schemas/")
		if seen[name] {
			t.Fatalf("schema reference cycle at %q", name)
		}
		seen[name] = true

		for property := range propertiesOf(t, document, document.Components.Schemas[name], seen) {
			properties[property] = true
		}
	}

	for property := range node.Properties {
		properties[property] = true
	}

	for _, part := range node.AllOf {
		for property := range propertiesOf(t, document, part, seen) {
			properties[property] = true
		}
	}

	return properties
}

// jsonNames is the set of names encoding/json sends for a struct: its json
// tags, with an anonymous embedded struct flattened into its parent.
func jsonNames(typ reflect.Type) map[string]bool {
	names := map[string]bool{}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if field.Anonymous && name == "" {
			for embedded := range jsonNames(field.Type) {
				names[embedded] = true
			}
			continue
		}

		if name == "" || name == "-" {
			continue
		}

		names[name] = true
	}

	return names
}
