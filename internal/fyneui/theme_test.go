//go:build fyne

package fyneui

import (
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// A disabled field draws its value in the disabled colour, so the stock
// near-background shade made it unreadable. It must meet WCAG AA for normal
// text against the background, and still be dimmer than the normal foreground:
// a disabled value reads as inactive, not as a second heading.
func TestDisabledTextIsReadableAndMuted(t *testing.T) {
	brand := newBrandTheme()

	tests := []struct {
		name    string
		variant fyne.ThemeVariant
	}{
		{"light", theme.VariantLight},
		{"dark", theme.VariantDark},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			background := brand.Color(theme.ColorNameBackground, tt.variant)
			disabled := brand.Color(theme.ColorNameDisabled, tt.variant)
			foreground := brand.Color(theme.ColorNameForeground, tt.variant)

			if contrast := contrastRatio(disabled, background); contrast < 4.5 {
				t.Errorf("disabled text scores %.2f:1 against the background, want at least 4.5:1", contrast)
			}

			if contrastRatio(disabled, background) >= contrastRatio(foreground, background) {
				t.Error("disabled text is as strong as the normal foreground; it must stay muted")
			}
		})
	}
}

// contrastRatio is the WCAG 2.1 contrast between two opaque colours.
func contrastRatio(a, b color.Color) float64 {
	first, second := relativeLuminance(a), relativeLuminance(b)

	if first < second {
		first, second = second, first
	}

	return (first + 0.05) / (second + 0.05)
}

func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()

	return 0.2126*linearChannel(r) + 0.7152*linearChannel(g) + 0.0722*linearChannel(b)
}

func linearChannel(value uint32) float64 {
	channel := float64(value) / 0xFFFF

	if channel <= 0.04045 {
		return channel / 12.92
	}

	return math.Pow((channel+0.055)/1.055, 2.4)
}
