//go:build fyne

package fyneui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// aboutContent is the window's identity and its update controls.
func (u *ui) aboutContent() fyne.CanvasObject {
	logo := canvas.NewImageFromResource(logoResource())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(96, 96))

	name := widget.NewLabelWithStyle("eznit", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	name.SizeName = theme.SizeNameHeadingText

	version := widget.NewLabelWithStyle("Version "+u.updater.Current(), fyne.TextAlignCenter, fyne.TextStyle{})
	version.Selectable = true
	version.Importance = widget.LowImportance

	summary := widget.NewLabel("Your project's setup, in one place you host. " +
		"Templates, scripts and files you can pull onto any machine.")
	summary.Alignment = fyne.TextAlignCenter
	summary.Wrapping = fyne.TextWrapWord
	summary.Selectable = true

	var homepage fyne.CanvasObject

	if parsed, err := url.Parse(projectURL); err == nil {
		link := widget.NewHyperlink(projectURL, parsed)
		link.Alignment = fyne.TextAlignCenter
		homepage = link
	} else {
		label := widget.NewLabel(projectURL)
		label.Alignment = fyne.TextAlignCenter
		homepage = label
	}

	status := widget.NewLabel("Updates are checked when the window opens.")
	status.Wrapping = fyne.TextWrapWord
	status.Alignment = fyne.TextAlignCenter
	status.Selectable = true

	// Declared first so the check handler can name them; they are created just
	// below, before the window is shown.
	var install *widget.Button
	var restart *widget.Button
	var restore *widget.Button

	check := widget.NewButtonWithIcon("Check for updates", theme.ViewRefreshIcon(), func() {
		u.checkForUpdates(status, install, restore, restart)
	})

	install = widget.NewButtonWithIcon("Install", theme.DownloadIcon(), func() {
		u.installLatest(status, install, restore, restart)
	})
	install.Importance = widget.HighImportance
	install.Hide()

	restart = widget.NewButtonWithIcon("Restart now", theme.ViewRefreshIcon(), func() {
		if err := u.updater.Restart(); err != nil {
			status.SetText("The new version could not be started: " + err.Error())
			return
		}

		u.app.Quit()
	})
	restart.Importance = widget.SuccessImportance
	restart.Hide()

	if previous, ok := u.updater.Previous(); ok {
		restore = widget.NewButton("Restore "+previous, func() {
			u.restorePrevious(previous, status, install, restore, restart)
		})
		restore.Importance = widget.WarningImportance
	}

	actions := []fyne.CanvasObject{check, install, restart}
	if restore != nil {
		actions = append(actions, restore)
	}

	updates := widget.NewCard("Updates",
		"Checks GitHub for the newest release. Nothing is replaced without confirmation.",
		container.NewVBox(append([]fyne.CanvasObject{status}, actions...)...))

	body := container.NewVBox(
		container.NewCenter(logo),
		name,
		version,
		summary,
		container.NewCenter(homepage),
		widget.NewSeparator(),
		updates,
	)

	// The panel is a page: it scrolls, and it fills the width so the summary
	// and the card wrap with the window.
	return container.NewVScroll(container.NewPadded(body))
}

// checkForUpdates is the startup courtesy: one lookup, in the background.
func (u *ui) checkForUpdates(status *widget.Label, install, restore, restart *widget.Button) {
	status.SetText("Checking for updates…")

	go func() {
		latest, err := u.updater.Latest()

		fyne.Do(func() {
			// The other buttons are not touched by a check; the arguments
			// exist so the panel has one shape for all four handlers.
			_ = restore
			_ = restart

			switch {
			case err != nil:
				status.SetText("Could not check for updates: " + err.Error())
				install.Hide()
			case latest == "":
				status.SetText("No release information was returned.")
				install.Hide()
			case latest == u.updater.Current():
				status.SetText("eznit " + u.updater.Current() + " is the newest release.")
				install.Hide()
			default:
				status.SetText("eznit " + latest + " is available (you have " + u.updater.Current() + ").")
				install.SetText("Install " + latest)
				install.Show()
			}
		})
	}()
}

// installLatest looks the newest release up again before asking, so a stale
// startup answer cannot install the wrong tag.
func (u *ui) installLatest(status *widget.Label, install, restore, restart *widget.Button) {
	status.SetText("Looking up the newest release…")

	go func() {
		latest, err := u.updater.Latest()

		fyne.Do(func() {
			if err != nil {
				status.SetText("Could not check for updates: " + err.Error())
				return
			}

			if latest == "" || latest == u.updater.Current() {
				status.SetText("eznit " + u.updater.Current() + " is the newest release.")
				install.Hide()
				return
			}

			dialog.ShowConfirm("Install "+latest,
				"Install eznit "+latest+"?\n\nIt replaces the binary on disk; a restart finishes the update.",
				func(confirmed bool) {
					if !confirmed {
						return
					}

					status.SetText("Installing eznit " + latest + "…")

					go func() {
						err := u.updater.Install(latest)

						fyne.Do(func() {
							if err != nil {
								status.SetText("Install failed: " + err.Error())
								return
							}

							status.SetText("Installed eznit " + latest + ". Restart to use it.")
							install.Hide()
							restart.Show()
							if restore != nil {
								restore.Hide()
							}
						})
					}()
				}, u.window)
		})
	}()
}

// restorePrevious puts back the version saved by the last install.
func (u *ui) restorePrevious(previous string, status *widget.Label, install, restore, restart *widget.Button) {
	dialog.ShowConfirm("Restore "+previous,
		"Restore eznit "+previous+"?\n\nIt replaces the binary on disk; a restart finishes the change.",
		func(confirmed bool) {
			if !confirmed {
				return
			}

			status.SetText("Restoring eznit " + previous + "…")

			go func() {
				err := u.updater.Restore()

				fyne.Do(func() {
					if err != nil {
						status.SetText("Restore failed: " + err.Error())
						return
					}

					status.SetText("Restored eznit " + previous + ". Restart to use it.")
					if install != nil {
						install.Hide()
					}
					if restore != nil {
						restore.Hide()
					}
					restart.Show()
				})
			}()
		}, u.window)
}
