// Package crower reads and writes crower command packs: the TOML files
// `crower export` produces and `crower import` consumes.
//
// The format is crower's, not eznit's. A pack is one table per command, keyed
// by the name the command is invoked by; the fields inside are crower's
// (Exec, Steps, aliases, shell, environment). eznit stores a command as a
// script instead, so the conversion is deliberately narrow:
//
//   - to crower: the script content becomes Exec;
//   - from crower: Steps are what runs when they are present — crower ignores
//     Exec then — so they become the script, one step per line; otherwise Exec
//     does.
//
// Everything else (aliases, env, shell, cwd, timeout, icon, group) is carried
// through the file but has no eznit column: the CLI reports it, the pack keeps
// it, and a future change does not have to guess.
package crower

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	toml "github.com/pelletier/go-toml/v2"
)

// Command is one entry of a pack. The field names are crower's: it writes
// `Name`, `AllAlias`, `Exec` and the optional metadata without lower-casing
// them, and a file that renamed them would not load there.
type Command struct {
	Name string `toml:"Name,omitempty"`

	Aliases []string `toml:"AllAlias,omitempty"`
	Exec    string   `toml:"Exec,omitempty"`
	Steps   []string `toml:"Steps,omitempty"`

	Description string            `toml:"Description,omitempty"`
	Params      []string          `toml:"Params,omitempty"`
	Icon        string            `toml:"Icon,omitempty"`
	Group       string            `toml:"Group,omitempty"`
	Shell       string            `toml:"Shell,omitempty"`
	Cwd         string            `toml:"Cwd,omitempty"`
	Env         map[string]string `toml:"Env,omitempty"`
	Timeout     string            `toml:"Timeout,omitempty"`
}

// ReadFile loads a pack. The table key is the command's name: crower itself
// makes the key win over the stored Name, because every other part of crower
// indexes by the key, so a file whose two disagree cannot import one name into
// two.
func ReadFile(path string) ([]Command, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the pack %s: %w", path, err)
	}

	var pack map[string]Command
	if err := toml.Unmarshal(data, &pack); err != nil {
		return nil, fmt.Errorf("the pack %s is not valid TOML: %w", path, err)
	}

	commands := make([]Command, 0, len(pack))
	for key, command := range pack {
		if key == "" {
			return nil, fmt.Errorf("the pack %s holds a command with an empty name", path)
		}

		command.Name = key
		commands = append(commands, command)
	}

	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })

	return commands, nil
}

// WriteFile writes the commands as a pack, keyed by name. The file is written
// to a temporary name and renamed, so a failed write does not leave half a
// pack for crower to import.
func WriteFile(path string, commands []Command) error {
	pack := make(map[string]Command, len(commands))

	for _, command := range commands {
		if command.Name == "" {
			return fmt.Errorf("a command with an empty name cannot go into a pack")
		}
		if _, exists := pack[command.Name]; exists {
			return fmt.Errorf("the pack would hold %q twice", command.Name)
		}
		pack[command.Name] = command
	}

	var buffer bytes.Buffer
	encoder := toml.NewEncoder(&buffer)

	if err := encoder.Encode(pack); err != nil {
		return fmt.Errorf("encode the pack: %w", err)
	}

	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".eznit-pack-*")
	if err != nil {
		return fmt.Errorf("create the pack next to %s: %w", path, err)
	}

	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if _, err := temporary.Write(buffer.Bytes()); err != nil {
		temporary.Close()
		return fmt.Errorf("write the pack: %w", err)
	}

	if err := temporary.Chmod(0644); err != nil {
		temporary.Close()
		return fmt.Errorf("set the pack permissions: %w", err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close the pack: %w", err)
	}

	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("move the pack into place: %w", err)
	}

	return nil
}
