package cli

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
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

// A binary file is not left out: its bytes travel as base64 and the server
// writes them to disk. A text column could not hold them.
func TestCollectTemplateFilesSendsTheBinaryFilesAsBase64(t *testing.T) {
	root := t.TempDir()

	content := []byte("the bytes\x00and the rest")

	writeTestFile(t, filepath.Join(root, "AGENTS.md"), "the project")
	writeTestFile(t, filepath.Join(root, "logo.png"), string(content))

	files, skipped, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(skipped) != 0 {
		t.Errorf("expected nothing to be skipped, got %v", skipped)
	}

	if len(files) != 2 {
		t.Fatalf("expected both files, got %v", templateFilePaths(files))
	}

	byPath := map[string]templateapply.File{}
	for _, file := range files {
		byPath[file.Path] = file
	}

	binary, ok := byPath["logo.png"]
	if !ok {
		t.Fatalf("expected the binary file, got %v", templateFilePaths(files))
	}

	if binary.Kind != templates.FileKindBinary {
		t.Errorf("expected kind %q, got %q", templates.FileKindBinary, binary.Kind)
	}

	if binary.Content != "" {
		t.Errorf("expected no text content for a binary file, got %q", binary.Content)
	}

	if binary.ContentBase64 != base64.StdEncoding.EncodeToString(content) {
		t.Errorf("expected the bytes as base64, got %q", binary.ContentBase64)
	}

	if text := byPath["AGENTS.md"]; text.Kind != templates.FileKindText || text.Content != "the project" {
		t.Errorf("expected the text file to travel as text, got %+v", text)
	}
}

// A byte that is not valid UTF-8 is not text either, even without a NUL.
func TestCollectTemplateFilesTreatsInvalidUTF8AsBinary(t *testing.T) {
	root := t.TempDir()

	writeTestFile(t, filepath.Join(root, "latin1.txt"), "caf\xe9")

	files, _, err := collectTemplateFiles(root)
	if err != nil {
		t.Fatalf("expected the walk to succeed, got error %v", err)
	}

	if len(files) != 1 || files[0].Kind != templates.FileKindBinary {
		t.Fatalf("expected the file to be sent as binary, got %+v", files)
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
		content := templateapply.Substitute(file.Content, values)

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
		{templates.KindBash, "shell/bashrc.sh"},
		{templates.KindZsh, "shell/zshrc.sh"},
		{templates.KindFish, "shell/config.fish"},
		{templates.KindPowershell, "shell/profile.ps1"},
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

// A shell starter is about the shell: it writes the file the shell sources
// and nothing else, with the project's placeholders in it.
func TestShellStarterWritesOnlyItsShellFile(t *testing.T) {
	cases := []struct {
		kind string
		path string
		want string // a fragment only that shell's file has
	}{
		{templates.KindBash, "shell/bashrc.sh", "~/.bashrc"},
		{templates.KindZsh, "shell/zshrc.sh", "~/.zshrc"},
		{templates.KindFish, "shell/config.fish", "~/.config/fish/config.fish"},
		{templates.KindPowershell, "shell/profile.ps1", "$PROFILE"},
	}

	values := map[string]string{
		"PROJECT_NAME":        "eznit",
		"PROJECT_DESCRIPTION": "self-hosted setup",
		"LANGUAGE":            "Go",
	}

	for _, tt := range cases {
		t.Run(tt.kind, func(t *testing.T) {
			files := starterTemplate(tt.kind)

			if len(files) != 1 {
				t.Fatalf("expected one file, got %v", templateFilePaths(files))
			}
			if files[0].Path != tt.path {
				t.Errorf("path = %q, want %q", files[0].Path, tt.path)
			}

			content := templateapply.Substitute(files[0].Content, values)

			if strings.Contains(content, "{{") {
				t.Errorf("a placeholder was not filled in:\n%s", content)
			}
			if !strings.Contains(content, "eznit") {
				t.Errorf("the project name is missing:\n%s", content)
			}
			if !strings.Contains(content, tt.want) {
				t.Errorf("the file does not name %q:\n%s", tt.want, content)
			}
		})
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
func templateFilePaths(files []templateapply.File) []string {
	paths := make([]string, 0, len(files))

	for _, file := range files {
		paths = append(paths, file.Path)
	}

	return paths
}
