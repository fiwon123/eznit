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

// The bundle is one binary now, and a folder from an older release carries the
// client *and* the standalone server. The name alone cannot say which one can
// serve, so the binary is asked — and a client that predates `serve` is skipped
// in favour of the server beside it, which is the shape an in-place update
// leaves behind.
func TestServerBinaryChoosesTheOneThatCanServe(t *testing.T) {
	writeBinary := func(dir, name, body string) {
		t.Helper()

		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// A merged binary answers `serve --help`; a client from before the merge
	// does not.
	const serves = "#!/bin/sh\nexit 0\n"
	const onlyClient = "#!/bin/sh\nexit 2\n"
	const legacy = "#!/bin/sh\nexit 0\n"

	t.Run("the merged binary wins", func(t *testing.T) {
		dir := t.TempDir()
		writeBinary(dir, BinaryName(), serves)
		writeBinary(dir, legacyBinaryName(), legacy)

		path, args, err := ServerBinary(dir)
		if err != nil {
			t.Fatalf("ServerBinary() error = %v", err)
		}
		if path != filepath.Join(dir, BinaryName()) {
			t.Errorf("ServerBinary() = %q, want %s", path, BinaryName())
		}
		if len(args) != 1 || args[0] != ServerCommand {
			t.Errorf("args = %q, want [%s]", args, ServerCommand)
		}
	})

	t.Run("an old bundle keeps its server", func(t *testing.T) {
		// The shape every release before the merge produced: a client that
		// cannot serve, and the standalone server beside it.
		dir := t.TempDir()
		writeBinary(dir, BinaryName(), onlyClient)
		writeBinary(dir, legacyBinaryName(), legacy)

		path, args, err := ServerBinary(dir)
		if err != nil {
			t.Fatalf("ServerBinary() error = %v, want the standalone server", err)
		}
		if path != filepath.Join(dir, legacyBinaryName()) {
			t.Errorf("ServerBinary() = %q, want %s", path, legacyBinaryName())
		}
		if len(args) != 0 {
			t.Errorf("args = %q, want none for the standalone server", args)
		}
	})

	t.Run("only the standalone server", func(t *testing.T) {
		dir := t.TempDir()
		writeBinary(dir, legacyBinaryName(), legacy)

		path, _, err := ServerBinary(dir)
		if err != nil {
			t.Fatalf("ServerBinary() error = %v", err)
		}
		if path != filepath.Join(dir, legacyBinaryName()) {
			t.Errorf("ServerBinary() = %q, want the standalone server", path)
		}
	})

	t.Run("a client too old to serve, and nothing else", func(t *testing.T) {
		dir := t.TempDir()
		writeBinary(dir, BinaryName(), onlyClient)

		_, _, err := ServerBinary(dir)
		if err == nil || !strings.Contains(err.Error(), "download the release again") {
			t.Errorf("ServerBinary() error = %v, want it to say the binary is too old", err)
		}
	})

	t.Run("an empty folder", func(t *testing.T) {
		_, _, err := ServerBinary(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), ServerBinaryName) {
			t.Errorf("ServerBinary() error = %v, want it to name %s", err, ServerBinaryName)
		}
	})
}

// Inspect answers from the names on disk and runs nothing: the panel calls it
// on whatever folder a person points at.
func TestInspectRunsNothing(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, BinaryName()), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, EnvExampleName), []byte("API_PORT=4000\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if bundle := Inspect(dir); !bundle.Binary {
		t.Error("Inspect() did not see the binary that cannot serve")
	}
}
