package config

import (
	"os"
	"path/filepath"
	"testing"
)

func useTempStore(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv(Env, path)

	return path
}

func TestReadMissingFileIsEmpty(t *testing.T) {
	useTempStore(t)

	values, err := Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(values) != 0 {
		t.Errorf("values = %v, want an empty map", values)
	}
}

func TestSetKeepsOtherValues(t *testing.T) {
	useTempStore(t)

	if err := Set("token", "abc"); err != nil {
		t.Fatalf("Set(token) error = %v", err)
	}
	if err := Set("api_url", "http://example.test"); err != nil {
		t.Fatalf("Set(api_url) error = %v", err)
	}

	values, err := Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if values["token"] != "abc" || values["api_url"] != "http://example.test" {
		t.Errorf("values = %v, want both keys", values)
	}
}

func TestValueAndRemove(t *testing.T) {
	useTempStore(t)

	if _, err := Value("token"); err == nil {
		t.Fatal("Value(missing) error = nil, want an error")
	}

	if err := Set("token", "abc"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if value, err := Value("token"); err != nil || value != "abc" {
		t.Errorf("Value() = %q, %v, want abc", value, err)
	}

	if err := Remove("token"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := Value("token"); err == nil {
		t.Error("Value() after Remove = nil error, want the key gone")
	}
}

// The file holds credentials: it must not be world-readable, even when it
// already existed with a looser mode.
func TestWriteTightensTheMode(t *testing.T) {
	path := useTempStore(t)

	if err := Set("token", "abc"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := Set("token", "def"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestInvalidJSONIsAnError(t *testing.T) {
	path := useTempStore(t)

	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := Read(); err == nil {
		t.Fatal("Read() error = nil, want the invalid file refused")
	}
}
