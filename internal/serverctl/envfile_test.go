package serverctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The repository's own example is the file a bundle user copies, so it is the
// one a save has to round-trip: values equal, comments kept, order kept.
func TestEnvFileRoundTripsTheExample(t *testing.T) {
	source := filepath.Join("..", "..", EnvExampleName)

	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, EnvFileName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	before, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	if err := before.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	after, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv after save: %v", err)
	}

	if got, want := strings.Join(after.ChildEnv(), "\n"), strings.Join(before.ChildEnv(), "\n"); got != want {
		t.Errorf("values changed across a save:\n got %q\nwant %q", got, want)
	}

	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, comment := range []string{"# API", "# DB", "# Web UI", "# Auth rate limiting"} {
		if !strings.Contains(string(saved), comment) {
			t.Errorf("the saved file lost the comment %q", comment)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %v, want 0600 — a .env can hold a database password", mode)
	}
}

func TestEnvFileReadsTheValuesTheApiReads(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		value string
	}{
		{"double quoted", `API_PORT="4000"`, "4000"},
		{"single quoted", `API_PORT='4000'`, "4000"},
		{"unquoted", "API_PORT=4000", "4000"},
		{"empty quotes", `API_PORT=""`, ""},
		{"an escaped quote", `DB_PWD="a\"b"`, `a"b`},
		{"a backslash", `DB_PWD="a\\b"`, `a\b`},
		{"a newline escape", `DB_PWD="a\nb"`, "a\nb"},
		{"an export line", `export API_PORT="4000"`, "4000"},
		{"spaces around the equals", `API_PORT = "4000"`, "4000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), EnvFileName)
			if err := os.WriteFile(path, []byte(tt.line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			file, err := LoadEnv(path)
			if err != nil {
				t.Fatalf("LoadEnv: %v", err)
			}

			got, ok := file.Get("API_PORT")
			if !ok {
				got, ok = file.Get("DB_PWD")
			}
			if !ok {
				t.Fatalf("%q was not read as an assignment", tt.line)
			}
			if got != tt.value {
				t.Errorf("value = %q, want %q", got, tt.value)
			}
		})
	}
}

// A value a person types has to survive the trip through the file: quotes and
// backslashes included.
func TestEnvFileRoundTripsAValueWithSpecialCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), EnvFileName)

	file, err := LoadEnv(path)
	if err == nil {
		t.Fatal("LoadEnv read a file that does not exist")
	}

	file = &EnvFile{path: path}
	value := `pa"ss\word` + "\t" + "x"

	file.Set("DB_PWD", value)
	if err := file.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	if got, _ := reloaded.Get("DB_PWD"); got != value {
		t.Errorf("value = %q, want %q", got, value)
	}
}

func TestEnvFileSetAddsAndUnsetRemoves(t *testing.T) {
	path := filepath.Join(t.TempDir(), EnvFileName)
	if err := os.WriteFile(path, []byte("# a comment\nAPI_PORT=\"4000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, err := LoadEnv(path)
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}

	file.Set("API_PORT", "5000")
	file.Set("DB_HOST", "127.0.0.1")
	file.Unset("API_PORT")

	if _, ok := file.Get("API_PORT"); ok {
		t.Error("API_PORT is still there after Unset")
	}
	if got := file.Value("DB_HOST", ""); got != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want %q", got, "127.0.0.1")
	}
	if got := file.Value("API_LOGS", "./logs"); got != "./logs" {
		t.Errorf("Value() fallback = %q, want %q", got, "./logs")
	}
}

// The first run on a fresh bundle: no .env, but the example is there. The copy
// has to point at the local database and at the bundle's secret file, or the
// server cannot reach either.
func TestLoadOrCreateCopiesTheExampleForALocalServer(t *testing.T) {
	dir := t.TempDir()
	example := "# API\nAPI_PORT=\"4000\"\n\n# DB\nDB_HOST=\"db\"\nDB_PWD=\"throwaway\"\nDB_NAME=\"eznit\"\n"
	if err := os.WriteFile(filepath.Join(dir, EnvExampleName), []byte(example), 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}

	if got := file.Value("DB_HOST", ""); got != "127.0.0.1" {
		t.Errorf("DB_HOST = %q, want %q for a server on this machine", got, "127.0.0.1")
	}
	if got := file.Value("DB_PASSWORD_FILE", ""); got != "./"+SecretPath {
		t.Errorf("DB_PASSWORD_FILE = %q, want %q", got, "./"+SecretPath)
	}
	if _, ok := file.Get("DB_PWD"); ok {
		t.Error("DB_PWD is still set, so it would win over the secret file")
	}
	if got := file.Value("API_PORT", ""); got != "4000" {
		t.Errorf("API_PORT = %q, want the example's %q", got, "4000")
	}

	if _, err := os.Stat(filepath.Join(dir, EnvFileName)); err != nil {
		t.Errorf("the .env was not created: %v", err)
	}

	// A second run reads the file it created instead of copying again.
	again, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatalf("second LoadOrCreate: %v", err)
	}
	if got := again.Value("DB_HOST", ""); got != "127.0.0.1" {
		t.Errorf("DB_HOST after the second load = %q", got)
	}
}

func TestLoadOrCreateRefusesAFolderThatIsNotABundle(t *testing.T) {
	if _, err := LoadOrCreate(t.TempDir()); err == nil {
		t.Fatal("LoadOrCreate accepted a folder with neither .env nor .env.example")
	}
}

func TestWriteSecretWritesPrivateFile(t *testing.T) {
	dir := t.TempDir()

	if err := WriteSecret(dir, "  s3cret  "); err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}

	path := filepath.Join(dir, filepath.FromSlash(SecretPath))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the secret: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != "s3cret" {
		t.Errorf("the file holds %q, want the trimmed password", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %v, want 0600", mode)
	}

	exists, mode, err := SecretState(dir)
	if err != nil {
		t.Fatalf("SecretState: %v", err)
	}
	if !exists || mode != 0o600 {
		t.Errorf("SecretState = (%v, %v), want (true, 0600)", exists, mode)
	}
}

// A file that exists group-readable has to be tightened, not left as it was:
// os.WriteFile keeps the mode of a file that is already there.
func TestWriteSecretTightensAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, filepath.FromSlash(SecretPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := WriteSecret(dir, "new"); err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %v, want 0600 after a rewrite", mode)
	}
}

func TestWriteSecretRefusesAnEmptyPassword(t *testing.T) {
	if err := WriteSecret(t.TempDir(), "   "); err == nil {
		t.Fatal("WriteSecret accepted an empty password")
	}
}

func TestSecretStateWhenTheFileIsMissing(t *testing.T) {
	exists, _, err := SecretState(t.TempDir())
	if err != nil {
		t.Fatalf("SecretState: %v", err)
	}
	if exists {
		t.Error("SecretState found a secret in an empty folder")
	}
}
