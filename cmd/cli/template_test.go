package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// The templates routes are behind the session guard, so the CLI has to send
// the stored token with every request it makes on them.
func TestSendTemplatesRequestSendsTheSessionToken(t *testing.T) {
	var gotAuthorization string
	var gotPath string
	var gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery

		w.Write([]byte(`{"status_code":200,"data":{"data":[],"total":0}}`))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	items, ok := sendTemplatesRequest(server.URL, false, "opencode", "the-token", globals)
	if !ok {
		t.Fatal("expected the catalogue to be read")
	}

	if len(items) != 0 {
		t.Errorf("expected an empty catalogue, got %d templates", len(items))
	}

	if gotAuthorization != "Bearer the-token" {
		t.Errorf("expected the request to carry the token, got %q", gotAuthorization)
	}

	if gotPath != "/v1/templates" {
		t.Errorf("expected the catalogue route, got %q", gotPath)
	}

	if gotQuery != "kind=opencode" {
		t.Errorf("expected the kind to filter the catalogue, got %q", gotQuery)
	}
}

// The "mine" route is a different one, so it has to be the one that is
// called when the user asks for their own templates.
func TestSendTemplatesRequestUsesTheMineRoute(t *testing.T) {
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Write([]byte(`{"status_code":200,"data":{"data":[],"total":0}}`))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	if _, ok := sendTemplatesRequest(server.URL, true, "", "the-token", globals); !ok {
		t.Fatal("expected the catalogue to be read")
	}

	if gotPath != "/v1/templates/mine" {
		t.Errorf("expected the mine route, got %q", gotPath)
	}
}

// A publish carries the files and the questions, so the body has to hold
// them: the server holds the template, the client only holds the answers.
func TestSendPublishRequestSendsTheFilesAndTheVariables(t *testing.T) {
	var gotAuthorization string
	var gotPath string
	var gotBody templates.PublishRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("failed to decode the request body: %v", err)
		}

		w.Write([]byte(`{"status_code":200,"data":{"id":"11111111-1111-1111-1111-111111111111","name":"the template"}}`))
	}))
	defer server.Close()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	globals := newGlobals(&API{baseURL: server.URL}, t.TempDir(), log)

	request := templates.PublishRequest{
		Name:     "the template",
		Kind:     templates.KindOpencode,
		IsPublic: true,
		Files: []templates.PublishFile{
			{Path: "AGENTS.md", Content: "hello {{PROJECT_NAME}}"},
		},
		Variables: []templates.PublishVariable{
			{Key: "PROJECT_NAME", Prompt: "the name", Required: true},
		},
	}

	detail, ok := sendPublishRequest(server.URL, request, "the-token", globals)
	if !ok {
		t.Fatal("expected the template to be published")
	}

	if detail.Name != "the template" {
		t.Errorf("expected the published template to come back, got %q", detail.Name)
	}

	if gotAuthorization != "Bearer the-token" {
		t.Errorf("expected the request to carry the token, got %q", gotAuthorization)
	}

	if gotPath != "/v1/templates" {
		t.Errorf("expected the publish route, got %q", gotPath)
	}

	if len(gotBody.Files) != 1 || gotBody.Files[0].Path != "AGENTS.md" {
		t.Fatalf("expected the file to be sent, got %+v", gotBody.Files)
	}

	if len(gotBody.Variables) != 1 || gotBody.Variables[0].Key != "PROJECT_NAME" {
		t.Errorf("expected the variable to be sent, got %+v", gotBody.Variables)
	}
}

// The publish walk leaves out the folders that are not part of a project:
// the version control metadata and the dependency trees.
func TestCollectTemplateFilesSkipsTheNoiseFolders(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "the project")
	writeTestFile(t, filepath.Join(root, ".git", "config"), "[core]")
	writeTestFile(t, filepath.Join(root, "node_modules", "left-pad", "index.js"), "module.exports = 1")
	writeTestFile(t, filepath.Join(root, "src", "main.go"), "package main")

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(skipped) != 0 {
		t.Errorf("expected nothing to be skipped, got %v", skipped)
	}

	paths := templateFilePaths(files)

	want := []string{"AGENTS.md", "src/main.go"}

	if len(paths) != len(want) {
		t.Fatalf("expected %d files, got %d (%v)", len(want), len(paths), paths)
	}

	for i, path := range want {
		if paths[i] != path {
			t.Errorf("expected the file %q, got %q", path, paths[i])
		}
	}
}

// A template holds text: the contents live in a text column, and a byte that
// is not text does not survive the trip. The file is named, so the author
// knows the folder holds more than the template does.
func TestCollectTemplateFilesSkipsTheFilesThatAreNotText(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "the project")
	writeTestFile(t, filepath.Join(root, "logo.png"), "the bytes\x00and the rest")

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected only the text file, got %v", templateFilePaths(files))
	}

	if len(skipped) != 1 || skipped[0] != "logo.png" {
		t.Errorf("expected the binary file to be named, got %v", skipped)
	}
}

// A file that is too big is left out: the server refuses it, and a template
// that would be refused is not worth sending.
func TestCollectTemplateFilesSkipsTheFilesThatAreTooBig(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "the project")

	big := strings.Repeat("a", maxTemplateFileBytes+1)
	writeTestFile(t, filepath.Join(root, "big.txt"), big)

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected only the small file, got %v", templateFilePaths(files))
	}

	if len(skipped) != 1 || skipped[0] != "big.txt" {
		t.Errorf("expected the big file to be named, got %v", skipped)
	}
}

// A symlink can point outside the folder. A template is made of the files
// themselves, so a symlink is left out.
func TestCollectTemplateFilesSkipsTheSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a symlink needs a privilege windows does not give by default")
	}

	root := t.TempDir()

	outside := filepath.Join(t.TempDir(), "secret.txt")
	writeTestFile(t, outside, "the secret")

	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "the project")

	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("the symlink could not be created: %v", err)
	}

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected only the real file, got %v", templateFilePaths(files))
	}

	if len(skipped) != 1 || skipped[0] != "link.txt" {
		t.Errorf("expected the symlink to be named, got %v", skipped)
	}
}

// An executable file keeps its flag: a script that cannot be run is not the
// script the author published.
func TestCollectTemplateFilesKeepsTheExecutableFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable flag means nothing on windows")
	}

	root := t.TempDir()

	path := filepath.Join(root, "run.sh")
	writeTestFile(t, path, "#!/bin/sh\necho hello\n")

	if err := os.Chmod(path, 0755); err != nil {
		t.Fatalf("failed to set the mode: %v", err)
	}

	files, _, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected one file, got %v", templateFilePaths(files))
	}

	if !files[0].IsExecutable {
		t.Error("expected the executable flag to be kept")
	}
}

// The placeholders in the files are the questions the template asks. The
// name has the shape the server accepts, so a placeholder the CLI finds is
// a placeholder the server would accept.
func TestFindPlaceholders(t *testing.T) {
	files := []templateFile{
		{Path: "AGENTS.md", Content: "# {{PROJECT_NAME}}\n\n{{PROJECT_DESCRIPTION}}\n"},
		{Path: "src/main.go", Content: "// {{PROJECT_NAME}} again\n"},
		{Path: "notes.md", Content: "{{ lowercase }} and {{MIXED-case}}\n"},
	}

	keys := findPlaceholders(files)

	want := []string{"PROJECT_DESCRIPTION", "PROJECT_NAME"}

	if len(keys) != len(want) {
		t.Fatalf("expected %v, got %v", want, keys)
	}

	for i, key := range want {
		if keys[i] != key {
			t.Errorf("expected the key %q, got %q", key, keys[i])
		}
	}
}

// A placeholder that has a value is replaced. A placeholder that has none is
// left as it is: the template may use the braces for something else.
func TestSubstitute(t *testing.T) {
	content := "# {{PROJECT_NAME}}\n\n{{PROJECT_DESCRIPTION}}\n\n{{UNKNOWN}}\n"

	got := substitute(content, map[string]string{
		"PROJECT_NAME": "eznit",
	})

	want := "# eznit\n\n{{PROJECT_DESCRIPTION}}\n\n{{UNKNOWN}}\n"

	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// The placeholders the files hold that the template does not declare are
// named: the user can see what the template left out.
func TestUndeclaredPlaceholders(t *testing.T) {
	detail := &templates.TemplateDetail{
		Files: []templates.TemplateFileData{
			{Path: "AGENTS.md", Content: "{{PROJECT_NAME}} {{SECRET}} {{OTHER}}"},
		},
		Variables: []templates.TemplateVariableData{
			{Key: "PROJECT_NAME"},
		},
	}

	keys := undeclaredPlaceholders(detail)

	if len(keys) != 2 {
		t.Fatalf("expected two undeclared placeholders, got %v", keys)
	}

	if keys[0] != "OTHER" || keys[1] != "SECRET" {
		t.Errorf("expected the undeclared placeholders to be named, got %v", keys)
	}
}

// A template writes into a folder the user names, and the paths come from
// the server — from another user. A path that would land outside the folder
// is refused, and nothing is written.
func TestWriteTemplateFilesRefusesAPathOutsideTheTarget(t *testing.T) {
	target := t.TempDir()

	files := []templateFile{
		{Path: "AGENTS.md", Content: "the project"},
		{Path: "../escape.txt", Content: "the escape"},
	}

	err := writeTemplateFiles(target, files, false)
	if err == nil {
		t.Fatal("expected the path outside the folder to be refused")
	}

	if !strings.Contains(err.Error(), "outside") {
		t.Errorf("expected the error to name the reason, got %v", err)
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("failed to read the target folder: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("expected nothing to be written, found %d entries", len(entries))
	}
}

// A file that is already there is not overwritten: the user asks for it
// with --force.
func TestWriteTemplateFilesRefusesToOverwrite(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "AGENTS.md"), "the old one")

	files := []templateFile{
		{Path: "AGENTS.md", Content: "the new one"},
	}

	err := writeTemplateFiles(target, files, false)
	if err == nil {
		t.Fatal("expected the existing file to be protected")
	}

	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("expected the error to name the file, got %v", err)
	}

	content, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatalf("failed to read the file: %v", err)
	}

	if string(content) != "the old one" {
		t.Errorf("expected the file to be untouched, got %q", string(content))
	}
}

// With --force the file is written over.
func TestWriteTemplateFilesOverwritesWithForce(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "AGENTS.md"), "the old one")

	files := []templateFile{
		{Path: "AGENTS.md", Content: "the new one"},
	}

	if err := writeTemplateFiles(target, files, true); err != nil {
		t.Fatalf("expected the file to be written, got error %v", err)
	}

	content, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatalf("failed to read the file: %v", err)
	}

	if string(content) != "the new one" {
		t.Errorf("expected the new content, got %q", string(content))
	}
}

// The files are written with the folders they need, and an executable file
// is executable.
func TestWriteTemplateFilesWritesTheTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable flag means nothing on windows")
	}

	target := t.TempDir()

	files := []templateFile{
		{Path: "AGENTS.md", Content: "the project"},
		{Path: ".opencode/agents/build.md", Content: "the agent"},
		{Path: "run.sh", Content: "#!/bin/sh", IsExecutable: true},
	}

	if err := writeTemplateFiles(target, files, false); err != nil {
		t.Fatalf("expected the files to be written, got error %v", err)
	}

	content, err := os.ReadFile(filepath.Join(target, ".opencode", "agents", "build.md"))
	if err != nil {
		t.Fatalf("expected the nested file to be written: %v", err)
	}

	if string(content) != "the agent" {
		t.Errorf("expected the file content, got %q", string(content))
	}

	info, err := os.Stat(filepath.Join(target, "run.sh"))
	if err != nil {
		t.Fatalf("failed to stat the executable: %v", err)
	}

	if info.Mode()&0111 == 0 {
		t.Errorf("expected the file to be executable, got mode %v", info.Mode())
	}
}

// The starter is a template like any other: it holds the placeholders, and
// they are filled in with the answers.
func TestStarterTemplateHoldsThePlaceholders(t *testing.T) {
	files := starterTemplate(templates.KindOpencode)

	if len(files) != 4 {
		t.Fatalf("expected the starter to hold four files, got %v", templateFilePaths(files))
	}

	paths := templateFilePaths(files)

	for _, want := range []string{"AGENTS.md", ".opencode/agents/build.md", ".opencode/agents/plan.md", ".opencode/agents/reviewer.md"} {
		found := false

		for _, path := range paths {
			if path == want {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("expected the starter to hold %q, got %v", want, paths)
		}
	}

	values := map[string]string{
		"PROJECT_NAME":        "eznit",
		"PROJECT_DESCRIPTION": "self-hosted file storage",
		"LANGUAGE":            "Go",
	}

	for _, file := range files {
		content := substitute(file.Content, values)

		if strings.Contains(content, "{{") {
			t.Errorf("expected every placeholder of %q to be filled in, got %q", file.Path, content)
		}

		if !strings.Contains(content, "eznit") && file.Path == "AGENTS.md" {
			t.Errorf("expected the name to be written in %q, got %q", file.Path, content)
		}
	}
}

// Every terminal gets the AGENTS.md, because it is the file they all read.
// The ones with their own folder get it too.
func TestStarterTemplateForEveryKind(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{templates.KindGeneric, "AGENTS.md"},
		{templates.KindCodex, "AGENTS.md"},
		{templates.KindCopilot, ".github/copilot-instructions.md"},
		{templates.KindCursor, ".cursor/rules/project.mdc"},
		{templates.KindWindsurf, ".windsurf/rules/project.md"},
		{templates.KindOpencode, ".opencode/agents/build.md"},
	}

	for _, tt := range cases {
		t.Run(tt.kind, func(t *testing.T) {
			paths := templateFilePaths(starterTemplate(tt.kind))

			found := false

			for _, path := range paths {
				if path == tt.want {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("expected the starter to hold %q, got %v", tt.want, paths)
			}
		})
	}
}

// The files are checked before the questions are asked: a user is not asked
// what a template wants to know when the template is not going to be
// written.
func TestCheckTemplateFilesRefusesAnExistingFile(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "AGENTS.md"), "the old one")

	files := []templateFile{
		{Path: "AGENTS.md", Content: "the new one"},
	}

	err := checkTemplateFiles(target, files, false)
	if err == nil {
		t.Fatal("expected the existing file to be protected")
	}

	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("expected the error to name the file, got %v", err)
	}
}

// A path that would land outside the target is refused by the check, before
// anything is written.
func TestCheckTemplateFilesRefusesAPathOutsideTheTarget(t *testing.T) {
	target := t.TempDir()

	files := []templateFile{
		{Path: "../escape.txt", Content: "the escape"},
	}

	err := checkTemplateFiles(target, files, false)
	if err == nil {
		t.Fatal("expected the path outside the folder to be refused")
	}
}

// The kind filter is applied to the catalogue the "mine" route answers, so
// the flag means the same thing whichever route is used.
func TestFilterTemplatesByKind(t *testing.T) {
	items := []templates.TemplateData{
		{ID: uuid.New(), Name: "one", Kind: templates.KindOpencode},
		{ID: uuid.New(), Name: "two", Kind: templates.KindCodex},
	}

	filtered := filterTemplatesByKind(items, templates.KindCodex)

	if len(filtered) != 1 {
		t.Fatalf("expected one template, got %d", len(filtered))
	}

	if filtered[0].Name != "two" {
		t.Errorf("expected the codex template, got %q", filtered[0].Name)
	}

	if len(filterTemplatesByKind(items, "")) != 2 {
		t.Errorf("expected every template to be kept, got %d", len(filterTemplatesByKind(items, "")))
	}
}

// writeTestFile writes a file, with the folders it needs.
func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create the folder of %q: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %q: %v", path, err)
	}
}

// templateFilePaths returns the paths of a list of files.
func templateFilePaths(files []templateFile) []string {
	paths := make([]string, 0, len(files))

	for _, file := range files {
		paths = append(paths, file.Path)
	}

	return paths
}
