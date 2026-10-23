package sql

import (
	"os"
	"path/filepath"
	"testing"
)

// The DSN is one string built from five variables; a mistyped format verb
// here is a connection failure that looks like a database problem.
func TestDSNFromEnv(t *testing.T) {
	t.Setenv("DB_USER", "root")
	t.Setenv("DB_PASSWORD_FILE", "")
	t.Setenv("DB_PWD", "secret")
	t.Setenv("DB_HOST", "db")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_NAME", "eznit")

	want := "postgresql://root:secret@db:5432/eznit?sslmode=disable"
	if got := DSNFromEnv(); got != want {
		t.Errorf("DSNFromEnv() = %q, want %q", got, want)
	}
}

// The local stack passes the password as a Docker secret, so a file wins over
// the inline variable: an inline DB_PWD left in a stale .env must not quietly
// take over from the secret.
func TestPasswordFromFileWins(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "db_password")
	if err := os.WriteFile(passwordFile, []byte("from-the-file\n"), 0o600); err != nil {
		t.Fatalf("write the password file: %v", err)
	}

	t.Setenv("DB_PASSWORD_FILE", passwordFile)
	t.Setenv("DB_PWD", "from-the-environment")

	if got := PasswordFromEnv(); got != "from-the-file" {
		t.Errorf("PasswordFromEnv() = %q, want the file's password", got)
	}
}

// A DB_PASSWORD_FILE that is not there must not leave the DSN without a
// password: the fallback is the inline variable.
func TestPasswordFallsBackToTheEnvironment(t *testing.T) {
	t.Setenv("DB_PASSWORD_FILE", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("DB_PWD", "from-the-environment")

	if got := PasswordFromEnv(); got != "from-the-environment" {
		t.Errorf("PasswordFromEnv() = %q, want the environment's password", got)
	}
}
