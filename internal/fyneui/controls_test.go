//go:build fyne

package fyneui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// findTapTarget returns the first tap layer in the tree. A tap layer is a
// widget, not a container, so it is found directly.
func findTapTarget(root fyne.CanvasObject) *tapTarget {
	var found *tapTarget

	walkTree(root, fyne.NewPos(0, 0), func(object fyne.CanvasObject, _ fyne.Position) {
		if target, ok := object.(*tapTarget); ok && found == nil {
			found = target
		}
	})

	return found
}

// The caption of a checkbox is a label, so it can be copied; the check itself
// carries no text of its own, and a tap on the caption still toggles the box.
func TestCheckRowCaptionIsSelectableAndStillToggles(t *testing.T) {
	var changed []bool

	item := newCheckRow("Only my templates", func(checked bool) { changed = append(changed, checked) })

	if item.Text != "" {
		t.Errorf("the check paints its own text %q; the caption should be the label", item.Text)
	}

	if item.label.Text != "Only my templates" {
		t.Errorf("caption = %q, want the checkbox's text", item.label.Text)
	}

	if !item.label.Selectable {
		t.Error("the caption is not selectable")
	}

	tap := findTapTarget(item.row)
	if tap == nil {
		t.Fatal("the caption has no tap layer, so clicking it would do nothing")
	}

	test.Tap(tap)

	if !item.Checked {
		t.Error("a tap on the caption did not check the box")
	}

	test.Tap(tap)

	if item.Checked {
		t.Error("a second tap on the caption did not uncheck the box")
	}

	if len(changed) != 2 || changed[0] != true || changed[1] != false {
		t.Errorf("OnChanged saw %v, want [true false]", changed)
	}
}

// A disabled checkbox ignores a tap on its caption, exactly as the box itself
// does.
func TestCheckRowIgnoresTapsWhenDisabled(t *testing.T) {
	item := newCheckRow("Follow output", nil)
	item.Disable()

	test.Tap(findTapTarget(item.row))

	if item.Checked {
		t.Error("a disabled checkbox was checked by a tap on its caption")
	}
}

// A tap layer consumes taps only: it must not claim drags, or selecting a
// label's text underneath would be impossible.
func TestTapTargetOnlyAcceptsTaps(t *testing.T) {
	target := newTapTarget(func() {})

	if _, ok := any(target).(fyne.Draggable); ok {
		t.Error("the tap layer accepts drags; a drag must reach the label's selection")
	}

	if _, ok := any(target).(fyne.Focusable); ok {
		t.Error("the tap layer takes focus, which belongs to the selectable label")
	}
}
