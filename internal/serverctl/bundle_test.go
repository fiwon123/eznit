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
