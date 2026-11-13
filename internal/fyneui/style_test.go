//go:build fyne

package fyneui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// One primary action is what makes a tab read as a page rather than a pile of
// buttons. The Server tab has two because its two panes are two views with one
// action each — Start on Output, Save configuration on Configuration — and the
// Save button's importance is load-bearing for the #235 pixel guard.
func TestTabsHaveOnePrimaryAction(t *testing.T) {
	u := newTestUI(t)

	for name, index := range map[string]int{"files": 0, "templates": 1} {
		primary := 0

		for _, button := range allButtons(u.tabs.Items[index].Content) {
			if !button.Hidden && button.Importance == widget.HighImportance {
				primary++
			}
		}

		if primary != 1 {
			t.Errorf("the %s tab has %d primary buttons, want one", name, primary)
		}
	}

	for _, button := range allButtons(u.tabs.Items[2].Content) {
		if !button.Hidden && button.Importance == widget.HighImportance {
			t.Errorf("the About tab offers a primary action %q; it is a page, not a form", button.Text)
		}
	}
}

// A numeric column lines its digits up; a column of words stays leading.
func TestNumericColumnsAreRightAligned(t *testing.T) {
	label := widget.NewLabel("12")

	tests := []struct {
		name   string
		align  func(*widget.Label, int)
		column int
		want   fyne.TextAlign
	}{
		{"a file version", alignFileColumn, 1, fyne.TextAlignTrailing},
		{"a file name", alignFileColumn, 0, fyne.TextAlignLeading},
		{"a template version", alignTemplateColumn, 2, fyne.TextAlignTrailing},
		{"a template file count", alignTemplateColumn, 4, fyne.TextAlignTrailing},
		{"a template kind", alignTemplateColumn, 1, fyne.TextAlignLeading},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.align(label, tt.column)

			if label.Alignment != tt.want {
				t.Errorf("alignment = %v, want %v", label.Alignment, tt.want)
			}
		})
	}
}

// A section caption is a sub-heading, not body text that happens to be bold.
func TestSectionCaptionsAreSubHeadings(t *testing.T) {
	built, ok := section("Variables", newProse("")).(*fyne.Container)
	if !ok {
		t.Fatal("a section is not a container")
	}

	title, ok := built.Objects[0].(*widget.Label)
	if !ok {
		t.Fatal("a section does not begin with its caption")
	}

	if title.SizeName != theme.SizeNameSubHeadingText {
		t.Errorf("caption size = %q, want the sub-heading size", title.SizeName)
	}

	if !title.TextStyle.Bold {
		t.Error("the caption is not bold")
	}
}
