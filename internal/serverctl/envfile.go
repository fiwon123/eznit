// Package serverctl is the local eznit server a desktop window runs: the .env
// it reads, the secret file it needs, and the server process itself.
//
// It imports neither a GUI toolkit nor the API, so its rules — what a start
// checks, what a stop means, what a save preserves — are tested without a
// display and without a database.
package serverctl

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// EnvFileName is the file the API reads its configuration from.
	EnvFileName = ".env"

	// EnvExampleName is the template a bundle ships. It is the one env file
	// that is not a secret, and it is what a first run copies.
	EnvExampleName = ".env.example"

	// SecretPath is the database password, relative to the bundle folder. It
	// is the same file Compose mounts as the db_password secret.
	SecretPath = "secrets/db_password.txt"
)

// EnvFile is one .env, kept as its lines so a save preserves the comments and
// the order a person wrote instead of flattening the file into a map.
type EnvFile struct {
	path    string
	entries []envEntry
}

// envEntry is a raw line (a comment or a blank) or a key with its value.
type envEntry struct {
	raw   string // the line as read, when key is empty
	key   string
	value string
}

// LoadEnv reads one .env. A missing file is an error: the caller decides
// whether to create it from the example.
func LoadEnv(path string) (*EnvFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	file := &EnvFile{path: path}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if text == "" {
		return file, nil
	}

	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		key, value, ok := splitAssignment(line)
		if !ok {
			file.entries = append(file.entries, envEntry{raw: line})
			continue
		}

		file.entries = append(file.entries, envEntry{key: key, value: value})
	}

	return file, nil
}

// LoadOrCreate loads dir/.env, creating it from .env.example when it is
// missing. The copy is adjusted for a server started on this machine: the
// database is reached over localhost, and its password is the bundle's secret
// file rather than the Compose service network.
func LoadOrCreate(dir string) (*EnvFile, error) {
	file, err := LoadEnv(filepath.Join(dir, EnvFileName))
	if err == nil {
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	example, err := LoadEnv(filepath.Join(dir, EnvExampleName))
	if err != nil {
		return nil, fmt.Errorf("there is no %s and no %s to copy — this folder is not a server bundle", EnvFileName, EnvExampleName)
	}

	example.path = filepath.Join(dir, EnvFileName)
	example.Set("DB_HOST", "127.0.0.1")
	example.Set("DB_PASSWORD_FILE", "./"+SecretPath)
	example.Unset("DB_PWD")

	if err := example.Save(); err != nil {
		return nil, err
	}

	return example, nil
}

// Path is where the file lives.
func (f *EnvFile) Path() string { return f.path }

// Get returns a value and whether the key is there. A key that is present and
// empty reads as present: an empty API_PORT is the API's business, not ours.
func (f *EnvFile) Get(key string) (string, bool) {
	for _, entry := range f.entries {
		if entry.key != "" && entry.key == key {
			return entry.value, true
		}
	}

	return "", false
}

// Value is Get with a fallback for a key that is not there.
func (f *EnvFile) Value(key, fallback string) string {
	if value, ok := f.Get(key); ok {
		return value
	}

	return fallback
}

// Set replaces the first line for key, or appends one.
func (f *EnvFile) Set(key, value string) {
	for i := range f.entries {
		if f.entries[i].key == key {
			f.entries[i].value = value
			return
		}
	}

	f.entries = append(f.entries, envEntry{key: key, value: value})
}

// Unset removes every line for key.
func (f *EnvFile) Unset(key string) {
	kept := f.entries[:0]

	for _, entry := range f.entries {
		if entry.key == key {
			continue
		}
		kept = append(kept, entry)
	}

	f.entries = kept
}

// ChildEnv is the file as the variables a child process inherits.
func (f *EnvFile) ChildEnv() []string {
	pairs := make([]string, 0, len(f.entries))

	for _, entry := range f.entries {
		if entry.key != "" {
			pairs = append(pairs, entry.key+"="+entry.value)
		}
	}

	return pairs
}

// Save writes the file back with the lines it came with. The mode is 0600: a
// .env holds a database password in the DB_PWD form, and the project keeps its
// local env files private.
func (f *EnvFile) Save() error {
	var text strings.Builder

	for _, entry := range f.entries {
		if entry.key == "" {
			text.WriteString(entry.raw)
		} else {
			text.WriteString(entry.key + "=" + quoteValue(entry.value))
		}
		text.WriteString("\n")
	}

	return writeFileAtomic(f.path, []byte(text.String()), 0o600)
}

// splitAssignment reads one `KEY=value` line. A comment, a blank line, or a
// line with no `=` is not an assignment and is preserved as it was.
func splitAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}

	key, rest, found := strings.Cut(trimmed, "=")
	if !found {
		return "", "", false
	}

	key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "export "))
	if key == "" {
		return "", "", false
	}

	return key, unquote(strings.TrimSpace(rest)), true
}

// unquote removes the matching quotes around a value. Double quotes take the
// backslash escapes godotenv understands; single quotes are literal.
func unquote(value string) string {
	if len(value) < 2 {
		return value
	}

	switch {
	case value[0] == '"' && value[len(value)-1] == '"':
		return strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\n`, "\n", `\r`, "\r").Replace(value[1 : len(value)-1])
	case value[0] == '\'' && value[len(value)-1] == '\'':
		return value[1 : len(value)-1]
	}

	return value
}

// quoteValue writes a value the way the example file does: double quotes, with
// the four characters that would otherwise end the string escaped.
func quoteValue(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`).Replace(value) + `"`
}

// WriteSecret writes the database password to dir/secrets/db_password.txt with
// mode 0600. The value is never returned, logged, or stored anywhere else: the
// file is the one place it lives.
func WriteSecret(dir, password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return errors.New("the database password is empty")
	}

	path := filepath.Join(dir, filepath.FromSlash(SecretPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(password+"\n"), 0o600); err != nil {
		return err
	}

	// WriteFile keeps the mode of a file that already existed, so a file that
	// was once created group-readable stays that way unless it is chmodded.
	return os.Chmod(path, 0o600)
}

// SecretState reports whether the password file is there and what mode it has.
// It never reads the value.
func SecretState(dir string) (exists bool, mode os.FileMode, err error) {
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(SecretPath)))
	if errors.Is(err, os.ErrNotExist) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}

	return true, info.Mode().Perm(), nil
}

// writeFileAtomic writes a sibling temporary file and renames it over the
// target, so a crash mid-save cannot leave a half-written .env behind.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)

	temp, err := os.CreateTemp(dir, ".env-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Close(); err != nil {
		return err
	}

	return os.Rename(temp.Name(), path)
}
