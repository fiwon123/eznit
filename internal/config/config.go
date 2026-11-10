// Package config is the local state shared by the eznit binaries: the session
// token, the git-host tokens and where the API is.
//
// It is one JSON file of string values. Nothing in it is sent to the eznit API
// except the session token — which travels as the Authorization header, not as
// a request body — and the host tokens never leave this machine.
//
// EZNIT_CONFIG names the file (a full path). Without it the file is
// `~/.eznit/config.json`.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Env is the variable that points the store at another file.
const Env = "EZNIT_CONFIG"

// Path is where the store lives.
func Path() (string, error) {
	if path, ok := os.LookupEnv(Env); ok && path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("the home folder is not known: %w", err)
	}

	return filepath.Join(home, ".eznit", "config.json"), nil
}

// Read returns the whole file. A file that is not there is an empty
// configuration, not an error: the first command that writes creates it. A
// file that cannot be parsed is an error — treating it as empty would drop
// every credential the user stored.
func Read() (map[string]string, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}

		return nil, fmt.Errorf("failed to read the config file: %w", err)
	}

	values := map[string]string{}
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}

	return values, nil
}

// Write replaces the file, and it keeps every key the caller did not touch:
// two binaries that both store something must not overwrite each other.
func Write(values map[string]string) error {
	path, err := Path()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create the config folder: %w", err)
	}

	raw, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode the config file: %w", err)
	}

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("failed to write the config file: %w", err)
	}

	// WriteFile does not change the mode of a file that already exists. A
	// credential file that is readable by others is a leak, so the mode is
	// set on every write.
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("failed to set the config file mode: %w", err)
	}

	return nil
}

// Value returns one value, or an error naming the key when it is not set. The
// error never contains the value.
func Value(key string) (string, error) {
	values, err := Read()
	if err != nil {
		return "", err
	}

	value, ok := values[key]
	if !ok || value == "" {
		return "", fmt.Errorf("%s is not set in the config file", key)
	}

	return value, nil
}

// Set stores one value and leaves the others alone.
func Set(key string, value string) error {
	values, err := Read()
	if err != nil {
		return err
	}

	values[key] = value

	return Write(values)
}

// Remove drops one value and leaves the others alone. It is not an error to
// remove a key that is not there.
func Remove(key string) error {
	values, err := Read()
	if err != nil {
		return err
	}

	delete(values, key)

	return Write(values)
}
