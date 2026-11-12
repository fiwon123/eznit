//go:build fyne

package fyneui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/fiwon123/eznit/internal/serverctl"
)

// statusKind is the state a status dot reports.
type statusKind int

const (
	statusIdle statusKind = iota
	statusPending
	statusOK
	statusWarn
	statusError
)

// The two palettes are not one palette at two brightnesses: each colour is
// picked to stay readable on the background it is drawn on.
var statusPalette = map[fyne.ThemeVariant]map[statusKind]color.Color{
	theme.VariantDark: {
		statusIdle:    color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF},
		statusPending: color.NRGBA{R: 0xFB, G: 0xBF, B: 0x24, A: 0xFF},
		statusOK:      color.NRGBA{R: 0x4A, G: 0xDE, B: 0x80, A: 0xFF},
		statusWarn:    color.NRGBA{R: 0xFB, G: 0xBF, B: 0x24, A: 0xFF},
		statusError:   color.NRGBA{R: 0xF8, G: 0x71, B: 0x71, A: 0xFF},
	},
	theme.VariantLight: {
		statusIdle:    color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF},
		statusPending: color.NRGBA{R: 0xB4, G: 0x53, B: 0x09, A: 0xFF},
		statusOK:      color.NRGBA{R: 0x15, G: 0x80, B: 0x3D, A: 0xFF},
		statusWarn:    color.NRGBA{R: 0xB4, G: 0x53, B: 0x09, A: 0xFF},
		statusError:   color.NRGBA{R: 0xB9, G: 0x1C, B: 0x1C, A: 0xFF},
	},
}

// statusColour is the dot's fill for a state.
func statusColour(variant fyne.ThemeVariant, kind statusKind) color.Color {
	palette, ok := statusPalette[variant]
	if !ok {
		palette = statusPalette[theme.VariantLight]
	}

	if colour, ok := palette[kind]; ok {
		return colour
	}

	return palette[statusIdle]
}

// logColour is the text colour for one level, per theme variant. A nil colour
// means the theme's own foreground, so ordinary lines look like the rest of
// the window.
var logColour = map[fyne.ThemeVariant]map[serverctl.Level]color.Color{
	theme.VariantDark: {
		serverctl.LevelDebug: color.NRGBA{R: 0x94, G: 0xA3, B: 0xB8, A: 0xFF},
		serverctl.LevelWarn:  color.NRGBA{R: 0xFB, G: 0xBF, B: 0x24, A: 0xFF},
		serverctl.LevelError: color.NRGBA{R: 0xF8, G: 0x71, B: 0x71, A: 0xFF},
	},
	theme.VariantLight: {
		serverctl.LevelDebug: color.NRGBA{R: 0x64, G: 0x74, B: 0x8B, A: 0xFF},
		serverctl.LevelWarn:  color.NRGBA{R: 0xB4, G: 0x53, B: 0x09, A: 0xFF},
		serverctl.LevelError: color.NRGBA{R: 0xB9, G: 0x1C, B: 0x1C, A: 0xFF},
	},
}

// logStyle is the grid style for one log line: a per-level text colour, and
// bold for the two levels that want attention.
func logStyle(variant fyne.ThemeVariant, line serverctl.LogLine) *widget.CustomTextGridStyle {
	colours, ok := logColour[variant]
	if !ok {
		colours = logColour[theme.VariantLight]
	}

	bold := line.Level == serverctl.LevelWarn || line.Level == serverctl.LevelError

	return &widget.CustomTextGridStyle{
		FGColor:   colours[line.Level],
		TextStyle: fyne.TextStyle{Bold: bold},
	}
}
