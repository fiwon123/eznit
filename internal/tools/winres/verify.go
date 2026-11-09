package main

import (
	"fmt"
	"os"
)

// verifyPath checks a built executable against what its name promises: a GUI
// binary is a GUI program carrying the icon, manifest and version; a console
// binary keeps its console, and carries the same resources when the build
// generated them.
func verifyPath(path string, gui, requireResources bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := Inspect(file)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if gui {
		if err := info.VerifyGUI(); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		return nil
	}

	if err := info.VerifyConsole(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	if requireResources {
		if err := info.VerifyResources(); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	return nil
}
