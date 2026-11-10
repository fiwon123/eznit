//go:build fyne

package fyneui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// The brand is one accent colour and a roomy layout. The sizes are theme
// values rather than per-widget overrides: a spacer added on top of the
// theme's padding is a double gap, and it is how a UI starts looking uneven.
const (
	// #2563EB, split into channels so the colour is one value to change.
	brandR = 0x25
	brandG = 0x63
	brandB = 0xEB

	appTextSize float32 = 15
	appPadding  float32 = 8
)

// brandTheme is the default theme with the eznit accent and a type scale that
// stays readable on a 1080p screen.
type brandTheme struct {
	fyne.Theme
}

func newBrandTheme() fyne.Theme {
	return brandTheme{Theme: theme.DefaultTheme()}
}

func (t brandTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	accent := color.NRGBA{R: brandR, G: brandG, B: brandB, A: 0xFF}

	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return accent
	case theme.ColorNameFocus:
		return color.NRGBA{R: 0x93, G: 0xC5, B: 0xFD, A: 0xFF}
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0x93, G: 0xC5, B: 0xFD, A: 0x66}
	}

	return t.Theme.Color(name, variant)
}

func (t brandTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		return appTextSize
	case theme.SizeNamePadding:
		return appPadding
	}

	return t.Theme.Size(name)
}
