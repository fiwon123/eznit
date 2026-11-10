//go:build fyne

package fyneui

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// aboutContent is the window's identity: the logo, the version, where the
// project lives, and the way out of the session.
//
// The update controls land here in the updater issue; the shape is already the
// one they will use, so the panel does not move later.
func (u *ui) aboutContent() fyne.CanvasObject {
	logo := canvas.NewImageFromResource(logoResource())
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(96, 96))

	name := widget.NewLabelWithStyle("eznit", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	version := widget.NewLabelWithStyle("Version "+u.version, fyne.TextAlignCenter, fyne.TextStyle{})
	version.Selectable = true

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

	signOut := widget.NewButtonWithIcon("Sign out", theme.CancelIcon(), u.signOut)
	signOut.Importance = widget.LowImportance

	body := container.NewVBox(
		container.NewCenter(logo),
		name,
		version,
		summary,
		container.NewCenter(homepage),
		widget.NewSeparator(),
		container.NewCenter(signOut),
	)

	return container.NewVScroll(container.NewPadded(body))
}
