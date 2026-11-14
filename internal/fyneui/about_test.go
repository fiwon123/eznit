//go:build fyne

package fyneui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// layOutAbout gives the About page a size, the way a window does, and returns
// the page's body. The tab is a scroll, so it is the scrolled content that gets
// the size — and the content, not the scroll, is what the walker can see into.
func layOutAbout(t *testing.T, u *ui) fyne.CanvasObject {
	t.Helper()

	scroll, ok := u.tabs.Items[2].Content.(*container.Scroll)
	if !ok {
		t.Fatalf("the About tab is %T, want a scroll", u.tabs.Items[2].Content)
	}

	about := scroll.Content
	about.Resize(fyne.NewSize(900, 600))
	relayout(about)

	return about
}

// Every word on About is meant to be copyable: the product name, the section
// heading, its explanation and the version are all labels, not painted text.
func TestAboutTextIsSelectable(t *testing.T) {
	u := newTestUI(t)
	about := layOutAbout(t, u)

	name := findLabel(about, "eznit")
	if name == nil {
		t.Fatal("the About tab does not show the product name")
	}

	if !name.Selectable {
		t.Error("the product name is not selectable")
	}

	updates := findLabel(about, "Updates")
	if updates == nil {
		t.Fatal("the About tab has no Updates heading")
	}

	if !updates.Selectable {
		t.Error("the Updates heading is not selectable")
	}

	if updates.Alignment != fyne.TextAlignCenter {
		t.Errorf("the Updates heading is %v-aligned, want centred", updates.Alignment)
	}

	if !updates.TextStyle.Bold {
		t.Error("the Updates heading is not a heading")
	}

	subtitle := findLabel(about, "Checks GitHub for the newest release. Nothing is replaced without confirmation.")
	if subtitle == nil {
		t.Fatal("the Updates section has no explanation")
	}

	if !subtitle.Selectable {
		t.Error("the Updates explanation is not selectable")
	}

	if subtitle.Alignment != fyne.TextAlignCenter {
		t.Errorf("the Updates explanation is %v-aligned, want centred", subtitle.Alignment)
	}

	if version := findLabel(about, "Version "+u.updater.Current()); version == nil {
		t.Fatal("the About tab does not show the version")
	} else if !version.Selectable {
		t.Error("the version is not selectable")
	}
}

// The action row is centred and its buttons keep their natural size: a VBox
// used to stretch every button across the card.
func TestAboutActionsAreCentredAtTheirNaturalSize(t *testing.T) {
	u := newTestUI(t)
	about := layOutAbout(t, u)

	heading := findLabel(about, "Updates")
	headingBox, ok := placeOf(about, heading)
	if !ok {
		t.Fatal("the Updates heading is not laid out")
	}

	centre := headingBox.pos.X + headingBox.size.Width/2

	left, right := float32(0), float32(0)
	seen := 0

	for _, button := range allButtons(about) {
		if button.Hidden {
			continue
		}

		box, ok := placeOf(about, button)
		if !ok {
			t.Fatalf("the %q button is not laid out", button.Text)
		}

		if box.size.Width > button.MinSize().Width+1 {
			t.Errorf("%q is %.1f px wide but needs %.1f — it is stretched, not natural",
				button.Text, box.size.Width, button.MinSize().Width)
		}

		if seen == 0 || box.pos.X < left {
			left = box.pos.X
		}

		if seen == 0 || box.pos.X+box.size.Width > right {
			right = box.pos.X + box.size.Width
		}

		seen++
	}

	if seen == 0 {
		t.Fatal("the About tab offers no action")
	}

	actionsCentre := (left + right) / 2
	if difference := actionsCentre - centre; difference > 1.5 || difference < -1.5 {
		t.Errorf("the action row centres at %.1f but the heading at %.1f — %.1f px apart",
			actionsCentre, centre, difference)
	}
}
