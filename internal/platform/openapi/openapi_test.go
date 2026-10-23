package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

// parse reads the embedded document as a generic tree. The tests navigate it
// by hand rather than through a typed struct: the point is to check the
// document as a document, and a struct would silently ignore the keys it does
// not name.
func parse(t *testing.T) map[string]any {
	t.Helper()

	var root map[string]any
	if err := yaml.Unmarshal(YAML(), &root); err != nil {
		t.Fatalf("the embedded document is not valid YAML: %v", err)
	}

	return root
}

func TestTheDocumentIsOpenAPI31(t *testing.T) {
	root := parse(t)

	if got := root["openapi"]; got != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", got)
	}

	info, ok := root["info"].(map[string]any)
	if !ok {
		t.Fatal("info is missing")
	}

	for _, key := range []string{"title", "version"} {
		if value, _ := info[key].(string); value == "" {
			t.Errorf("info.%s is empty", key)
		}
	}

	paths, ok := root["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("paths is missing or empty")
	}

	// Everything the API serves is under /v1; a path outside it would be the
	// web UI's, and this document is not about the web UI.
	for path := range paths {
		if !strings.HasPrefix(path, "/v1/") {
			t.Errorf("path %q is not under /v1", path)
		}
	}
}

// A reference that does not resolve is a broken link in the rendered UI, and
// it is invisible until somebody opens the page.
func TestEveryReferenceResolves(t *testing.T) {
	root := parse(t)

	var walk func(value any)
	walk = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && !resolves(root, ref) {
				t.Errorf("unresolved reference %q", ref)
			}

			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}

	walk(root)
}

func resolves(root map[string]any, ref string) bool {
	if !strings.HasPrefix(ref, "#/") {
		return false
	}

	var node any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		object, ok := node.(map[string]any)
		if !ok {
			return false
		}

		node, ok = object[part]
		if !ok {
			return false
		}
	}

	return true
}

// An operation a client cannot use is worse than a missing one: it renders in
// the UI and fails only when somebody tries it. Every operation needs an id, a
// summary, a success response and a declaration for each path parameter.
func TestEveryOperationIsComplete(t *testing.T) {
	root := parse(t)

	paths, _ := root["paths"].(map[string]any)
	seen := map[string]string{}
	placeholder := regexp.MustCompile(`\{([^}]+)\}`)

	for path, rawItem := range paths {
		item, ok := rawItem.(map[string]any)
		if !ok {
			t.Errorf("%s is not an object", path)
			continue
		}

		for method, rawOperation := range item {
			if !isMethod(method) {
				continue
			}

			operation, ok := rawOperation.(map[string]any)
			if !ok {
				t.Errorf("%s %s is not an object", method, path)
				continue
			}

			id, _ := operation["operationId"].(string)
			if id == "" {
				t.Errorf("%s %s has no operationId", method, path)
			} else if previous, duplicate := seen[id]; duplicate {
				t.Errorf("operationId %q is used by %s and %s %s", id, previous, method, path)
			} else {
				seen[id] = method + " " + path
			}

			if summary, _ := operation["summary"].(string); summary == "" {
				t.Errorf("%s %s has no summary", method, path)
			}

			responses, _ := operation["responses"].(map[string]any)
			if len(responses) == 0 {
				t.Errorf("%s %s has no responses", method, path)
			}

			success := false
			for status := range responses {
				if strings.HasPrefix(status, "2") || strings.HasPrefix(status, "3") {
					success = true
				}
			}
			if !success {
				t.Errorf("%s %s has no 2xx or 3xx response", method, path)
			}

			declared := parameterNames(t, root, item["parameters"])
			for name := range parameterNames(t, root, operation["parameters"]) {
				declared[name] = true
			}

			for _, match := range placeholder.FindAllStringSubmatch(path, -1) {
				if !declared[match[1]] {
					t.Errorf("%s %s uses {%s} but never declares it", method, path, match[1])
				}
			}
		}
	}
}

func isMethod(name string) bool {
	switch name {
	case "get", "put", "post", "delete", "patch", "head", "options", "trace":
		return true
	default:
		return false
	}
}

// parameterNames collects the `name` of every parameter in a list, following a
// `$ref` into components/parameters.
func parameterNames(t *testing.T, root map[string]any, raw any) map[string]bool {
	t.Helper()

	names := map[string]bool{}

	list, _ := raw.([]any)
	for _, entry := range list {
		parameter, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		if ref, ok := parameter["$ref"].(string); ok {
			resolved, ok := resolve(root, ref).(map[string]any)
			if !ok {
				t.Errorf("parameter reference %q does not resolve to an object", ref)
				continue
			}
			parameter = resolved
		}

		if name, _ := parameter["name"].(string); name != "" {
			names[name] = true
		}
	}

	return names
}

func resolve(root map[string]any, ref string) any {
	var node any = root
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		object, ok := node.(map[string]any)
		if !ok {
			return nil
		}

		node = object[part]
	}

	return node
}

func TestTheSpecIsServed(t *testing.T) {
	router := chi.NewRouter()
	Mount(router)

	t.Run("YAML", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, SpecPath, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", SpecPath, recorder.Code, http.StatusOK)
		}

		if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/yaml") {
			t.Errorf("Content-Type = %q, want application/yaml", contentType)
		}

		if body := recorder.Body.String(); body != string(YAML()) {
			t.Error("the served YAML is not the embedded document")
		}
	})

	t.Run("JSON", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, SpecJSONPath, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", SpecJSONPath, recorder.Code, http.StatusOK)
		}

		if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
			t.Errorf("Content-Type = %q, want application/json", contentType)
		}

		var document map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &document); err != nil {
			t.Fatalf("the served JSON does not parse: %v", err)
		}

		if got := document["openapi"]; got != "3.1.0" {
			t.Errorf("openapi = %v, want 3.1.0", got)
		}
	})
}

func TestTheDocsUIServesItsOwnAssets(t *testing.T) {
	router := chi.NewRouter()
	Mount(router)

	t.Run("a bare /docs redirects to the directory", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs", nil))

		if recorder.Code != http.StatusMovedPermanently {
			t.Fatalf("GET /docs = %d, want %d", recorder.Code, http.StatusMovedPermanently)
		}

		if location := recorder.Header().Get("Location"); location != DocsPath {
			t.Errorf("Location = %q, want %q", location, DocsPath)
		}
	})

	t.Run("the page loads the document and no network asset", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, DocsPath, nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want %d", DocsPath, recorder.Code, http.StatusOK)
		}

		if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", contentType)
		}

		body := recorder.Body.String()
		if !strings.Contains(body, SpecJSONPath) {
			t.Errorf("the page does not load %s", SpecJSONPath)
		}

		// The point of embedding the assets is that the reference works on a
		// host with no internet access.
		if strings.Contains(body, "https://") || strings.Contains(body, "http://") {
			t.Error("the page references a network URL; the UI must be self-contained")
		}
	})

	t.Run("an embedded asset is served", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/swagger-ui.css", nil))

		if recorder.Code != http.StatusOK {
			t.Fatalf("GET /docs/swagger-ui.css = %d, want %d", recorder.Code, http.StatusOK)
		}

		if recorder.Body.Len() == 0 {
			t.Error("the stylesheet is empty")
		}
	})

	t.Run("an unknown asset is not found", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/docs/nope.js", nil))

		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET /docs/nope.js = %d, want %d", recorder.Code, http.StatusNotFound)
		}
	})
}
