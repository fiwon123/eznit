package serverctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Inspect reads what a folder has of a bundle and names what is missing. It is
// the rule the Server tab asks before it offers Start or Create, so it is
// tested without a window.
func TestInspectBundle(t *testing.T) {
	binary := filepath.Join(t.TempDir(), BinaryName())
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	binaryDir := filepath.Dir(binary)

	exampleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(exampleDir, EnvExampleName), []byte("API_PORT=\"4000\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	bothDir := t.TempDir()
	for name, content := range map[string]string{
		BinaryName():   "#!/bin/sh\nexit 0\n",
		EnvExampleName: "API_PORT=\"4000\"\n",
	} {
		if err := os.WriteFile(filepath.Join(bothDir, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name    string
		dir     string
		want    Bundle
		missing []string
	}{
		{
			name:    "an empty folder",
			dir:     t.TempDir(),
			want:    Bundle{},
			missing: []string{BinaryName(), EnvExampleName},
		},
		{
			name:    "the binary only",
			dir:     binaryDir,
			want:    Bundle{Binary: true},
			missing: []string{EnvExampleName},
		},
		{
			name:    "the example only",
			dir:     exampleDir,
			want:    Bundle{Example: true},
			missing: []string{BinaryName()},
		},
		{
			name: "a whole bundle",
			dir:  bothDir,
			want: Bundle{Binary: true, Example: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Inspect(tt.dir)

			if got != tt.want {
				t.Errorf("Inspect() = %+v, want %+v", got, tt.want)
			}

			if got.IsBundle() != (tt.want.Binary && tt.want.Example) {
				t.Errorf("IsBundle() = %v for %+v", got.IsBundle(), got)
			}

			missing := strings.Join(got.Missing(), ",")
			if want := strings.Join(tt.missing, ","); missing != want {
				t.Errorf("Missing() = %q, want %q", missing, want)
			}
		})
	}
}

// Looking at a folder must not change it: the inspection runs every time the
// tab loads, including for folders a person only glanced at.
func TestInspectBundleWritesNothing(t *testing.T) {
	dir := t.TempDir()

	Inspect(dir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Errorf("Inspect created %d entries in an empty folder", len(entries))
	}
}

// The bundle carries one binary now, and a folder from an older release still
// holds the standalone server: it is used when `eznit` is absent, with no
// argument, so an archive somebody already extracted keeps starting.
func TestServerBinaryWithoutTheMergedName(t *testing.T) {
	dir := t.TempDir()

	writeBinary := func(name string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	_, _, err := ServerBinary(dir)
	if err == nil {
		t.Fatal("ServerBinary() = nil in an empty folder, want an error")
	}
	if !strings.Contains(err.Error(), ServerBinaryName) {
		t.Errorf("error = %q, want it to name %s", err, ServerBinaryName)
	}

	// An old bundle: only the standalone server.
	writeBinary(legacyBinaryName())

	path, args, err := ServerBinary(dir)
	if err != nil {
		t.Fatalf("ServerBinary() error = %v, want the legacy server", err)
	}
	if path != filepath.Join(dir, legacyBinaryName()) {
		t.Errorf("ServerBinary() = %q, want the legacy server", path)
	}
	if len(args) != 0 {
		t.Errorf("ServerBinary() args = %q, want none for the legacy server", args)
	}

	// A new bundle beside it: the merged binary wins, with its command.
	writeBinary(BinaryName())

	path, args, err = ServerBinary(dir)
	if err != nil {
		t.Fatalf("ServerBinary() error = %v", err)
	}
	if path != filepath.Join(dir, BinaryName()) {
		t.Errorf("ServerBinary() = %q, want %s to win", path, BinaryName())
	}
	if len(args) != 1 || args[0] != ServerCommand {
		t.Errorf("ServerBinary() args = %q, want [%s]", args, ServerCommand)
	}
}
