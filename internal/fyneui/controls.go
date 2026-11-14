//go:build fyne

package fyneui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// tapTarget is an invisible layer that turns a tap into a callback.
//
// Fyne hands a tap to the deepest object that accepts one, and a selectable
// label accepts it (its inner selection widget is Tappable as well as
// Draggable). Put a label behind something that has to react to a click and the
// click never arrives: the label's text selection swallows it.
//
// A tapTarget is stacked above the label and accepts only taps — not drags and
// not mouse-down — so the two gestures split cleanly. A tap reaches the layer
// and runs the callback; a drag reaches the label underneath and selects the
// text, which is the whole point of the label being selectable in the first
// place. Pointer moves still resolve to the label, so the cursor stays the text
// I-beam.
type tapTarget struct {
	widget.BaseWidget

	onTap func()
}

func newTapTarget(onTap func()) *tapTarget {
	target := &tapTarget{onTap: onTap}
	target.ExtendBaseWidget(target)

	return target
}

// Tapped runs the layer's callback. It is the only pointer interface the layer
// implements, on purpose: see the type comment.
func (t *tapTarget) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

// CreateRenderer draws nothing. A transparent rectangle still has a box, so
// the stack has something to lay out; painting is not what makes it tappable.
func (t *tapTarget) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// checkRow is a checkbox whose caption is a real label.
//
// widget.Check paints its own text with canvas.Text, which cannot be selected:
// the caption is part of the bitmap, not a label. This keeps the check itself
// (so Checked, SetChecked and OnChanged behave as before) and puts the caption
// beside it as a selectable widget.Label.
//
// The caption is also a click target: a tapTarget above it toggles the box the
// way clicking the caption of a checkbox normally does, while a drag still
// selects the label's text. Draw the row with row(), not with the embedded
// check, or the caption disappears.
type checkRow struct {
	*widget.Check

	label *widget.Label
	row   fyne.CanvasObject
}

func newCheckRow(text string, changed func(bool)) *checkRow {
	item := &checkRow{Check: widget.NewCheck("", changed)}

	item.label = widget.NewLabel(text)
	item.label.Selectable = true

	toggle := newTapTarget(func() {
		if item.Disabled() {
			return
		}

		item.SetChecked(!item.Checked)
	})

	item.row = container.NewHBox(item.Check, container.NewStack(item.label, toggle))

	return item
}
