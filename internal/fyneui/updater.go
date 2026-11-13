//go:build fyne

package fyneui

import (
	"os"
	"os/exec"

	"github.com/fiwon123/eznit/internal/update"
)

// appUpdater is the window's update seam: the About panel talks to this, and
// the package underneath owns the download, the checksum and the swap.
//
// The member is eznit-gui, not eznit: installing the CLI's bytes over the
// window turns the window into a program that prints help and exits.
type appUpdater struct {
	version string
	updater *update.Updater
}

func newAppUpdater(version string) *appUpdater {
	return &appUpdater{
		version: version,
		updater: update.New(update.MemberGUI),
	}
}

func (a *appUpdater) Current() string { return a.version }

func (a *appUpdater) Latest() (string, error) { return a.updater.Latest() }

func (a *appUpdater) Previous() (string, bool) { return a.updater.Previous() }

func (a *appUpdater) Install(version string) error {
	return a.updater.Install(a.version, version)
}

func (a *appUpdater) Restore() error { return a.updater.Restore() }

// Restart starts the binary on disk and lets the caller quit this process, so
// the new version runs with the old window closed.
func (a *appUpdater) Restart() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}

	return exec.Command(executable).Start()
}
