package crower

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPackRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.toml")

	want := []Command{
		{
			Name:        "deploy",
			Aliases:     []string{"dep", "d"},
			Exec:        "ssh prod 'cd app && git pull'",
			Description: "ship the app",
			Params:      []string{"branch"},
			Icon:        "🚀",
			Group:       "work",
			Shell:       "sh",
			Cwd:         "/srv",
			Env:         map[string]string{"BRANCH": "${1}"},
			Timeout:     "120s",
		},
		{
			Name: "fmt",
			Exec: "gofmt -w .",
		},
		{
			Name:  "greet",
			Steps: []string{"echo hello", "echo world"},
		},
	}

	if err := WriteFile(path, want); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	// ReadFile sorts by name; so does the comparison.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}
}

// The table key is the name crower indexes by; a stored Name that disagrees
// must not win.
func TestReadFileKeyWinsOverName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.toml")

	raw := `
[deploy]
Name = "shipping"
Exec = "echo hi"
`
	if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	commands, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(commands))
	}
	if commands[0].Name != "deploy" {
		t.Errorf("Name = %q, want the table key deploy", commands[0].Name)
	}
}

// A pack is crower's file: the top-level tables are the command names, not
// fields of one document.
func TestWriteFileUsesCrowerLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.toml")

	if err := WriteFile(path, []Command{{Name: "deploy", Exec: "echo hi"}}); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if !strings.Contains(string(body), "[deploy]") {
		t.Errorf("pack does not hold a [deploy] table:\n%s", body)
	}
	if !strings.Contains(string(body), `Exec = 'echo hi'`) {
		t.Errorf("pack does not hold the Exec field:\n%s", body)
	}
}

func TestReadFileRejectsInvalidTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.toml")
	if err := os.WriteFile(path, []byte("this is not = = toml"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := ReadFile(path); err == nil {
		t.Fatal("ReadFile() error = nil, want the invalid TOML refused")
	}
}

func TestWriteFileRejectsEmptyName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.toml")

	if err := WriteFile(path, []Command{{Exec: "echo hi"}}); err == nil {
		t.Fatal("WriteFile() error = nil, want the empty name refused")
	}
}

func TestWriteFileRejectsDuplicateName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.toml")

	err := WriteFile(path, []Command{
		{Name: "twice", Exec: "one"},
		{Name: "twice", Exec: "two"},
	})
	if err == nil {
		t.Fatal("WriteFile() error = nil, want the duplicate name refused")
	}
}
