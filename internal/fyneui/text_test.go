//go:build fyne

package fyneui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// wrappedExtent is the arithmetic behind prose's honest minimum: the widest
// word, and the lines the text takes at the width it is given.
func TestWrappedExtent(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)

	const textSize float32 = 15

	style := fyne.TextStyle{}
	space := fyne.MeasureText(" ", textSize, style).Width
	alpha := fyne.MeasureText("alpha", textSize, style).Width
	beta := fyne.MeasureText("beta", textSize, style).Width
	gamma := fyne.MeasureText("gamma", textSize, style).Width
	longest := fyne.Max(fyne.Max(alpha, beta), gamma)

	tests := []struct {
		name      string
		text      string
		available float32
		wantWidth float32
		wantLines int
	}{
		{"empty", "", 0, 0, 1},
		{"one word", "alpha", 0, alpha, 1},
		{"not laid out yet", "alpha beta", 0, fyne.Max(alpha, beta), 1},
		{"the words fit", "alpha beta", alpha + space + beta, fyne.Max(alpha, beta), 1},
		{"one pixel short", "alpha beta", alpha + space + beta - 1, fyne.Max(alpha, beta), 2},
		{"a narrow column", "alpha beta gamma", alpha + 1, longest, 3},
		{"explicit lines", "alpha\nbeta", 0, fyne.Max(alpha, beta), 2},
		{"a blank line", "alpha\n\nbeta", 0, fyne.Max(alpha, beta), 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, lines := wrappedExtent(tt.text, tt.available, textSize, style)

			if lines != tt.wantLines {
				t.Errorf("wrappedExtent(%q, %.1f) = %d lines, want %d", tt.text, tt.available, lines, tt.wantLines)
			}

			if width != tt.wantWidth {
				t.Errorf("wrappedExtent(%q, %.1f) width = %.1f, want %.1f", tt.text, tt.available, width, tt.wantWidth)
			}
		})
	}
}

// A prose label reports the width of its longest word, not the zero a wrapped
// Fyne label reports. That number is what stops a Border from squeezing it until
// a word is cut and what gives the window an honest minimum.
func TestProseMinimumIsTheLongestWord(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)

	text := "alpha beta gamma"
	inner := theme.InnerPadding()
	alpha := fyne.MeasureText("alpha", theme.TextSize(), fyne.TextStyle{}).Width

	wrapped := widget.NewLabel(text)
	wrapped.Wrapping = fyne.TextWrapWord
	wrappedModeless := wrapped.MinSize()

	label := newProse(text)
	got := label.MinSize()

	if got.Width < alpha+2*inner {
		t.Errorf("prose minimum width = %.1f, want at least the longest word (%.1f)", got.Width, alpha+2*inner)
	}

	if wrappedModeless.Width >= got.Width {
		t.Errorf("a plain wrapped label already reports %.1f px; the prose label (%.1f) adds nothing", wrappedModeless.Width, got.Width)
	}

	// Unlaid out it is one line; laid out narrow, the same text needs three.
	before := got.Height
	label.Resize(fyne.NewSize(alpha+4*inner, 0))
	after := label.MinSize().Height

	if after <= before {
		t.Errorf("prose height = %.1f at a narrow width, want more than the %.1f of one line", after, before)
	}
}
