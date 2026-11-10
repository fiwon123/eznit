package cli

import (
	"github.com/fiwon123/eznit/internal/config"
)

// The CLI keeps its local state in one file of its own: the session token and
// the provider credentials. The store lives in internal/config so the desktop
// app reads and writes the same file; these wrappers keep the command code and
// its tests on the names they already use.
//
// EZNIT_CONFIG names the file (a full path). Without it the file is
// `~/.eznit/config.json`.
const configEnv = config.Env

func configPath() (string, error) { return config.Path() }

func readConfig() (map[string]string, error) { return config.Read() }

func writeConfig(values map[string]string) error { return config.Write(values) }

func configValue(key string) (string, error) { return config.Value(key) }

func setConfigValue(key string, value string) error { return config.Set(key, value) }

func removeConfigValue(key string) error { return config.Remove(key) }
