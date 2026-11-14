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
	case theme.ColorNameDisabled:
		// The stock colour is a near-background shade (#E3E3E3 light,
		// #39393A dark), which is fine for a shape but not for text: a
		// disabled entry draws what it holds in this colour, so the address a
		// person needs to read became a blank field. These are slate greys —
		// still visibly secondary, at least 4.5:1 against the background in
		// both variants. The same colour is the disabled entry's border and a
		// disabled button's text, which is why both read again too.
		if variant == theme.VariantDark {
			return color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF}
		}

		return color.NRGBA{R: 0x64, G: 0x74, B: 0x8B, A: 0xFF}
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

// changedTheme tints an input that differs from the file it came from. It wraps
// the theme it was made from, so only the input colours move: a field marked
// here is a pending edit, not a different kind of field.
type changedTheme struct {
	fyne.Theme
}

func (t changedTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	dark := variant == theme.VariantDark

	switch name {
	case theme.ColorNameInputBackground:
		if dark {
			return color.NRGBA{R: 0x4A, G: 0x3B, B: 0x12, A: 0xFF}
		}

		return color.NRGBA{R: 0xFE, G: 0xF3, B: 0xC7, A: 0xFF}
	case theme.ColorNameInputBorder:
		if dark {
			return color.NRGBA{R: 0xFB, G: 0xBF, B: 0x24, A: 0xFF}
		}

		return color.NRGBA{R: 0xD9, G: 0x77, B: 0x06, A: 0xFF}
	}

	return t.Theme.Color(name, variant)
}
