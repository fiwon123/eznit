package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useConfig gives each test its own config file: no test reads or writes the
// real ~/.eznit.
func useConfig(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "eznit", "config.json")
	t.Setenv(configEnv, path)

	return path
}

// Two values written at two times both survive: the config file is shared by
// the session token and the provider token, and neither may erase the other.
func TestConfigKeepsEveryKey(t *testing.T) {
	path := useConfig(t)

	if err := setConfigValue("token", "session"); err != nil {
		t.Fatalf("failed to store the session token: %v", err)
	}

	if err := setConfigValue("github_token", "provider"); err != nil {
		t.Fatalf("failed to store the provider token: %v", err)
	}

	token, err := configValue("token")
	if err != nil || token != "session" {
		t.Errorf("expected the session token, got %q (%v)", token, err)
	}

	providerToken, err := configValue("github_token")
	if err != nil || providerToken != "provider" {
		t.Errorf("expected the provider token, got %q (%v)", providerToken, err)
	}

	// The credential file is not readable by anybody else.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat the config file: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected mode 0600, got %04o", mode)
	}
}

// A file that already exists keeps its own mode through a write, and
// WriteFile would leave a 0644 file readable: the mode is set on every write.
func TestConfigTightensAnExistingFile(t *testing.T) {
	path := useConfig(t)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("failed to create the config folder: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("failed to write the config file: %v", err)
	}

	if err := setConfigValue("token", "session"); err != nil {
		t.Fatalf("failed to store the token: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat the config file: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("expected mode 0600, got %04o", mode)
	}
}

// A file that cannot be parsed is an error, not an empty configuration: an
// empty one would drop every credential on the next write.
func TestConfigRefusesAMalformedFile(t *testing.T) {
	path := useConfig(t)

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("failed to create the config folder: %v", err)
	}

	const malformed = `{"token": "session"`

	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatalf("failed to write the config file: %v", err)
	}

	if _, err := readConfig(); err == nil {
		t.Fatal("expected the malformed file to be refused")
	}

	if _, err := configValue("token"); err == nil {
		t.Fatal("expected reading a value to fail")
	}

	// The file is still what it was; a failed read did not rewrite it.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read the config file: %v", err)
	}

	if string(raw) != malformed {
		t.Errorf("expected the file to be untouched, got %q", string(raw))
	}
}

// Removing one value leaves the others, and removing one that is not there is
// not an error.
func TestRemoveConfigValueKeepsTheRest(t *testing.T) {
	useConfig(t)

	if err := setConfigValue("token", "session"); err != nil {
		t.Fatalf("failed to store the session token: %v", err)
	}

	if err := setConfigValue("github_token", "provider"); err != nil {
		t.Fatalf("failed to store the provider token: %v", err)
	}

	if err := removeConfigValue("token"); err != nil {
		t.Fatalf("failed to remove the session token: %v", err)
	}

	if _, err := configValue("token"); err == nil {
		t.Error("expected the session token to be gone")
	}

	if providerToken, err := configValue("github_token"); err != nil || providerToken != "provider" {
		t.Errorf("expected the provider token to survive, got %q (%v)", providerToken, err)
	}

	if err := removeConfigValue("nothing"); err != nil {
		t.Errorf("expected removing a missing key to be fine, got %v", err)
	}
}

// eznit logout removes the session token, not the git-host credential: they
// are two different accounts.
func TestLogoutKeepsTheProviderToken(t *testing.T) {
	useConfig(t)

	if err := saveToken("session"); err != nil {
		t.Fatalf("failed to store the session token: %v", err)
	}

	if err := setConfigValue(providerTokenKey("github"), "provider"); err != nil {
		t.Fatalf("failed to store the provider token: %v", err)
	}

	if err := deleteToken(); err != nil {
		t.Fatalf("failed to remove the session token: %v", err)
	}

	if _, err := getToken(); err == nil {
		t.Error("expected the session token to be gone")
	}

	if token, err := configValue(providerTokenKey("github")); err != nil || token != "provider" {
		t.Errorf("expected the provider token to survive the logout, got %q (%v)", token, err)
	}
}

// A missing value is named, and the error never carries a stored value.
func TestConfigValueNamesTheMissingKey(t *testing.T) {
	useConfig(t)

	_, err := configValue("github_token")
	if err == nil {
		t.Fatal("expected an error for a missing value")
	}

	if !strings.Contains(err.Error(), "github_token") {
		t.Errorf("expected the key in the error, got %v", err)
	}
}

// The config file location follows EZNIT_CONFIG, so a container or a test
// profile does not have to touch the home folder.
func TestConfigPathFollowsTheEnvironment(t *testing.T) {
	path := useConfig(t)

	got, err := configPath()
	if err != nil {
		t.Fatalf("failed to resolve the config path: %v", err)
	}

	if got != path {
		t.Errorf("expected %q, got %q", path, got)
	}
}
