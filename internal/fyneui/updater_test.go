//go:build fyne

package fyneui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fiwon123/eznit/internal/update"
)

// The restart must start the path recorded before the swap. A fresh
// os.Executable() names the running inode, which the install renamed to
// "<binary>.old" — the backup, and the old version.
func TestRestartPathIsTheOneRecordedBeforeTheInstall(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "eznit-gui")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	running, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	if running == binary {
		t.Fatal("the test binary is the recorded path, so this test would prove nothing")
	}

	app := &appUpdater{version: "test", updater: &update.Updater{Executable: binary}}

	got, err := app.restartPath()
	if err != nil {
		t.Fatalf("restartPath() error = %v", err)
	}

	if got != binary {
		t.Errorf("restartPath() = %q, want the recorded %q", got, binary)
	}
}

// An install that never ran leaves no recorded path; the restart falls back to
// the running executable rather than failing.
func TestRestartPathFallsBackWhenTheRecordedPathIsGone(t *testing.T) {
	app := &appUpdater{
		version: "test",
		updater: &update.Updater{Executable: filepath.Join(t.TempDir(), "missing")},
	}

	got, err := app.restartPath()
	if err != nil {
		t.Fatalf("restartPath() error = %v", err)
	}

	if _, err := os.Stat(got); err != nil {
		t.Errorf("fallback %q does not exist: %v", got, err)
	}
}
