//go:build fyne

package fyneui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// prose is a label for text that wraps: a note, a status message, a template's
// description. It is a widget.Label with one difference — its minimum size is
// the truth.
//
// A Fyne label that wraps reports a minimum width of zero: the text renderer
// measures a line's width only when wrapping is off. Two things follow. A
// Border hands its middle whatever is left after its sides, which can be less
// than a word, and the label paints outside its box over whatever is beside it.
// And the window's own minimum comes from the same number, so the window can be
// resized into a state the tab cannot draw.
//
// prose measures the longest word for the width and the number of lines at the
// width it last had, so a row reserves the height the text actually takes and
// the window refuses to shrink below a word.
type prose struct {
	widget.Label
}

// newProse is a wrapped, selectable label: every value on these tabs is meant
// to be copyable.
func newProse(text string) *prose {
	label := &prose{}
	label.Text = text
	label.Wrapping = fyne.TextWrapWord
	label.Selectable = true
	label.ExtendBaseWidget(label)

	return label
}

// MinSize is the smallest box the text fits in: the longest word wide, and the
// height of the lines it takes at the width the label currently has.
//
// The width it needs is the largest word, never the width it happens to have:
// returning the latter is how a label agrees to be squeezed until a word is
// cut. The height is measured at the current width, which is why a row whose
// text changes has to be laid out again (see relayout) — the first pass still
// measures the old text.
func (p *prose) MinSize() fyne.Size {
	return p.requiredSize()
}

// requiredSize is what the text needs, independent of any widget bookkeeping.
// MinSize returns it, and the layout tests measure a laid-out box against it:
// a test that asked MinSize whether the box was big enough would only be asking
// the code under test whether it had worked.
func (p *prose) requiredSize() fyne.Size {
	inner := p.Theme().Size(theme.SizeNameInnerPadding)
	textSize := p.Theme().Size(theme.SizeNameText)
	lineHeight := fyne.MeasureText("Mg", textSize, p.TextStyle).Height
	spacing := p.Theme().Size(theme.SizeNameLineSpacing)

	available := p.Size().Width - 2*inner
	width, lines := wrappedExtent(p.Text, available, textSize, p.TextStyle)

	height := float32(lines) * lineHeight
	if lines > 1 {
		height += float32(lines-1) * spacing
	}

	return fyne.NewSize(width+2*inner, height+2*inner)
}

// wrappedExtent measures what a piece of text needs when it is wrapped: the
// width of its longest word, and how many lines it takes at available. An
// available width that is not positive means "not laid out yet", and then only
// the explicit lines count — the label has not been given a width to wrap at.
//
// It is separate from prose so the arithmetic can be tested without a widget.
func wrappedExtent(text string, available, textSize float32, style fyne.TextStyle) (float32, int) {
	longest := float32(0)
	lines := 0
	space := fyne.MeasureText(" ", textSize, style).Width

	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			// An empty line is a line of its own.
			lines++

			continue
		}

		lineWidth := float32(0)
		count := 1

		for _, word := range words {
			wordWidth := fyne.MeasureText(word, textSize, style).Width
			longest = fyne.Max(longest, wordWidth)

			if available <= 0 || lineWidth == 0 {
				lineWidth += wordWidth

				continue
			}

			if lineWidth+space+wordWidth > available {
				count++
				lineWidth = wordWidth

				continue
			}

			lineWidth += space + wordWidth
		}

		lines += count
	}

	if lines == 0 {
		lines = 1
	}

	return longest, lines
}

// oneLine is a selectable label for an identifier — an address, a folder, a
// health line — that must never be taller than one line or wider than its box.
// It ellipsizes instead: a path has no spaces to wrap at, so a wrapped label
// draws one long line straight over the buttons beside it.
func oneLine(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Truncation = fyne.TextTruncateEllipsis
	label.Selectable = true

	return label
}

// muted is secondary text: where an address came from, what a caption means. It
// stays selectable, because every value in this window can be copied.
func muted(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Importance = widget.LowImportance
	label.Selectable = true

	return label
}

// relayout lays a container out again after text that can change height has
// changed. Two passes, not one: a wrapped label reports the height it needs
// only once it has been laid out at its width, so the first pass gives the row
// the old height and the second gives it the new one. Call it after setting a
// note, a status or a selection — never from a goroutine, and not on every log
// line.
func relayout(object fyne.CanvasObject) {
	if object == nil {
		return
	}

	container, ok := object.(interface{ Refresh() })
	if !ok {
		return
	}

	container.Refresh()
	container.Refresh()
}
