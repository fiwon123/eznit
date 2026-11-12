package templateapply

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/google/uuid"
)

// writeTestFile puts a file where a test needs one already to exist.
func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create the folder of %q: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %q: %v", path, err)
	}
}

// The placeholders in the files are the questions a template asks. The name has
// the shape the server accepts, so a placeholder found here is one the server
// would accept, and anything else is left alone.
func TestUndeclaredFindsEveryPlaceholder(t *testing.T) {
	files := []File{
		{Path: "AGENTS.md", Content: "# {{PROJECT_NAME}}\n\n{{PROJECT_DESCRIPTION}}\n"},
		{Path: "src/main.go", Content: "// {{PROJECT_NAME}} again\n"},
		{Path: "notes.md", Content: "{{ lowercase }} and {{MIXED-case}}\n"},
	}

	keys := Undeclared(files, nil)

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

	got := Substitute(content, map[string]string{
		"PROJECT_NAME": "eznit",
	})

	want := "# eznit\n\n{{PROJECT_DESCRIPTION}}\n\n{{UNKNOWN}}\n"

	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// The placeholders the files hold that the template does not declare are named:
// a person can see what the template left out.
func TestUndeclaredSkipsTheDeclaredOnes(t *testing.T) {
	files := []File{
		{Path: "AGENTS.md", Content: "{{PROJECT_NAME}} {{SECRET}} {{OTHER}}"},
	}
	variables := []templates.TemplateVariableData{
		{Key: "PROJECT_NAME"},
	}

	keys := Undeclared(files, variables)

	if len(keys) != 2 {
		t.Fatalf("expected two undeclared placeholders, got %v", keys)
	}

	if keys[0] != "OTHER" || keys[1] != "SECRET" {
		t.Errorf("expected the undeclared placeholders to be named, got %v", keys)
	}
}

// A template writes into a folder the user names, and the paths come from the
// server — from another user. A path that would land outside the folder is
// refused, and nothing is written.
func TestWriteRefusesAPathOutsideTheTarget(t *testing.T) {
	target := t.TempDir()

	files := []File{
		{Path: "AGENTS.md", Content: "the project"},
		{Path: "../escape.txt", Content: "the escape"},
	}

	err := Write(target, files, false)
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

// A file that is already there is not overwritten: the caller asks for it.
func TestWriteRefusesToOverwrite(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "AGENTS.md"), "the old one")

	files := []File{
		{Path: "AGENTS.md", Content: "the new one"},
	}

	err := Write(target, files, false)
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

// With overwrite the file is written over.
func TestWriteOverwritesWhenAsked(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "AGENTS.md"), "the old one")

	files := []File{
		{Path: "AGENTS.md", Content: "the new one"},
	}

	if err := Write(target, files, true); err != nil {
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

// The files are written with the folders they need, and an executable file is
// executable.
func TestWriteCreatesFoldersAndExecutables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable flag means nothing on windows")
	}

	target := t.TempDir()

	files := []File{
		{Path: "AGENTS.md", Content: "the project"},
		{Path: ".opencode/agents/build.md", Content: "the agent"},
		{Path: "run.sh", Content: "#!/bin/sh", IsExecutable: true},
	}

	if err := Write(target, files, false); err != nil {
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

	if info.Mode()&0o111 == 0 {
		t.Errorf("expected the file to be executable, got mode %v", info.Mode())
	}
}

// The check runs before the questions are asked: a user is not asked what a
// template wants to know when the template is not going to be written.
func TestCheckRefusesAnExistingFile(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "AGENTS.md"), "the old one")

	files := []File{
		{Path: "AGENTS.md", Content: "the new one"},
	}

	err := Check(target, files, false)
	if err == nil {
		t.Fatal("expected the existing file to be protected")
	}

	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("expected the error to name the file, got %v", err)
	}

	if got := Check(target, files, true); got != nil {
		t.Errorf("Check(overwrite) = %v, want nil", got)
	}
}

func TestCheckRefusesAPathOutsideTheTarget(t *testing.T) {
	files := []File{
		{Path: "../escape.txt", Content: "the escape"},
	}

	if err := Check(t.TempDir(), files, false); err == nil {
		t.Fatal("expected the path outside the folder to be refused")
	}
}

// Two entries at one path would write one over the other, so the template is
// refused before anything is written.
func TestCheckRefusesADuplicatePath(t *testing.T) {
	files := []File{
		{Path: "scripts/run.sh", Content: "one"},
		{Path: "scripts/run.sh", Content: "two"},
	}

	if err := Check(t.TempDir(), files, false); err == nil {
		t.Fatal("expected the duplicate path to be refused")
	}
}

// Existing is what a caller shows before it overwrites anything: the paths
// that are already there, sorted, and nothing else.
func TestExistingListsWhatWouldBeOverwritten(t *testing.T) {
	target := t.TempDir()

	writeTestFile(t, filepath.Join(target, "b.txt"), "old")
	writeTestFile(t, filepath.Join(target, "a.txt"), "old")

	files := []File{
		{Path: "b.txt", Content: "new"},
		{Path: "c.txt", Content: "new"},
		{Path: "a.txt", Content: "new"},
	}

	existing, err := Existing(target, files)
	if err != nil {
		t.Fatal(err)
	}

	if len(existing) != 2 || existing[0] != "a.txt" || existing[1] != "b.txt" {
		t.Errorf("Existing = %v, want the two files that are there, sorted", existing)
	}

	empty, err := Existing(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("an empty folder reported %v", empty)
	}
}

// A binary file is decoded from the base64 the API sends, once, before anything
// is written.
func TestFilesDecodesBinaryContent(t *testing.T) {
	content := []byte{0x00, 0x01, 0xfe, 0xff}

	detail := &templates.TemplateDetail{
		Files: []templates.TemplateFileData{{
			Path:          "assets/logo.bin",
			Kind:          templates.FileKindBinary,
			Size:          int64(len(content)),
			ContentBase64: base64.StdEncoding.EncodeToString(content),
		}},
	}

	files, err := Files(detail)
	if err != nil {
		t.Fatalf("expected the files to be decoded, got error %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	if string(files[0].Bytes) != string(content) {
		t.Errorf("expected the bytes %v, got %v", content, files[0].Bytes)
	}
}

// A binary file the API sent without valid base64 is an error, not a file full
// of rubbish on disk.
func TestFilesRefusesInvalidBase64(t *testing.T) {
	detail := &templates.TemplateDetail{
		Files: []templates.TemplateFileData{{
			Path:          "assets/logo.bin",
			Kind:          templates.FileKindBinary,
			ContentBase64: "not base64!",
		}},
	}

	if _, err := Files(detail); err == nil {
		t.Fatal("expected invalid base64 to be refused")
	}
}

// A binary file is written with its bytes, and it keeps the executable flag.
func TestWriteWritesBinaryBytes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable flag means nothing on windows")
	}

	target := t.TempDir()
	content := []byte{0x7f, 'E', 'L', 'F', 0x00, 0x01}

	files := []File{{
		Path:         "bin/tool",
		Kind:         templates.FileKindBinary,
		Bytes:        content,
		IsExecutable: true,
	}}

	if err := Write(target, files, false); err != nil {
		t.Fatalf("expected the file to be written, got error %v", err)
	}

	path := filepath.Join(target, "bin", "tool")

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read the file: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("expected the bytes %v, got %v", content, got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat the file: %v", err)
	}

	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("expected the file to be executable, got mode %v", info.Mode())
	}
}

// A command a template pulls in is written like a file, at its target path.
func TestFilesIncludesTheCommands(t *testing.T) {
	content := []byte{0x7f, 'E', 'L', 'F', 0x00}

	detail := &templates.TemplateDetail{
		Files: []templates.TemplateFileData{{Path: "AGENTS.md", Kind: templates.FileKindText, Content: "# hi"}},
		Commands: []templates.TemplateCommandData{
			{
				CommandID:    uuid.New(),
				TargetPath:   "scripts/fmt.sh",
				Name:         "fmt",
				Kind:         templates.FileKindText,
				Content:      "#!/bin/sh\ngofmt -w .",
				IsExecutable: true,
			},
			{
				CommandID:     uuid.New(),
				TargetPath:    "bin/tool",
				Name:          "tool",
				Kind:          templates.FileKindBinary,
				Size:          int64(len(content)),
				ContentBase64: base64.StdEncoding.EncodeToString(content),
				IsExecutable:  true,
			},
		},
	}

	files, err := Files(detail)
	if err != nil {
		t.Fatalf("expected the files to be decoded, got error %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(files))
	}

	byPath := map[string]File{}
	for _, file := range files {
		byPath[file.Path] = file
	}

	if command := byPath["scripts/fmt.sh"]; command.Content != "#!/bin/sh\ngofmt -w ." || !command.IsExecutable {
		t.Errorf("expected the text command at its target path, got %+v", command)
	}

	if command := byPath["bin/tool"]; string(command.Bytes) != string(content) {
		t.Errorf("expected the binary command bytes, got %v", command.Bytes)
	}
}
