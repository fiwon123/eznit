package serverctl

import (
	"os"
	"path/filepath"
	"runtime"
)

// Bundle is what a folder carries of a server bundle: the executable the tab
// runs, and the example configuration a first run copies. A folder can have
// one without the other — a bundle someone already configured has no use for
// the example, and a copied folder can have the example without the binary.
type Bundle struct {
	Binary  bool
	Example bool
}

// IsBundle reports whether the folder could run a server from a first start:
// it has the binary and the example to make the configuration from.
func (b Bundle) IsBundle() bool {
	return b.Binary && b.Example
}

// Missing names the pieces the folder does not have, in the order they matter
// to a person reading the message.
func (b Bundle) Missing() []string {
	missing := make([]string, 0, 2)

	if !b.Binary {
		missing = append(missing, BinaryName())
	}

	if !b.Example {
		missing = append(missing, EnvExampleName)
	}

	return missing
}

// Inspect reports what dir has of a server bundle. It reads the folder and
// never writes to it: the window runs this before a person presses anything, so
// a wrong folder must not be changed by being looked at.
func Inspect(dir string) Bundle {
	_, binaryErr := ServerBinary(dir)
	_, exampleErr := os.Stat(filepath.Join(dir, EnvExampleName))

	return Bundle{Binary: binaryErr == nil, Example: exampleErr == nil}
}

// BinaryName is the server executable's name on this platform.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return ServerBinaryName + ".exe"
	}

	return ServerBinaryName
}
