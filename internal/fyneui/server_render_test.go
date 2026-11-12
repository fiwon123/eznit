//go:build fyne

package fyneui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// TestServerTabFitsTheConfiguration proves the panel is usable at the sizes a
// person actually gets: the configuration fields fit their pane without a
// scroll, and both panes render — the action row included — in both themes.
//
// It is the test that would have caught #235: the Save button used to sit at
// the bottom of the scrolling pane, below the fold, on a card that looked
// finished. And it is the test that guards the wheel: a Fyne entry keeps a
// scroll of its own and swallows the wheel over it, so the form must fit
// instead of scrolling at every size the window can be.
func TestServerTabFitsTheConfiguration(t *testing.T) {
	sizes := []fyne.Size{fyne.NewSize(980, 640), fyne.NewSize(1024, 720), fyne.NewSize(800, 520)}
	themes := map[string]fyne.Theme{"light": theme.LightTheme(), "dark": theme.DarkTheme()}

	for name, th := range themes {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%.0fx%.0f", name, size.Width, size.Height), func(t *testing.T) {
				dir := bundleDir(t, "API_PORT=\"4000\"\nAPI_SERVE_WEB=\"true\"\nAPI_UPLOADS=\"./uploads\"\nAPI_LOGS=\"./logs\"\nDB_HOST=\"127.0.0.1\"\nDB_PORT=\"5432\"\nDB_USER=\"root\"\nDB_NAME=\"eznit\"\n")

				view := newTestServerView(t, dir)
				view.ui.app.Settings().SetTheme(th)
				view.ui.window.Resize(size)

				// Resizing the window does not lay the content out until it is
				// shown, so lay it out explicitly and measure what a person
				// would see.
				view.ui.window.Content().Resize(size)

				// The form is reached by opening its tab, and a tab that is not
				// selected is not laid out.
				view.panes.SelectIndex(1)
				view.ui.window.Content().Resize(size)

				content, pane := view.configScroll.Content.MinSize(), view.configScroll.Size()
				if content.Height > pane.Height {
					t.Errorf("the configuration needs %.0f px of height but its pane has %.0f — the fields would scroll",
						content.Height, pane.Height)
				}
				if content.Width > pane.Width {
					t.Errorf("the configuration needs %.0f px of width but its pane has %.0f",
						content.Width, pane.Width)
				}

				// The actions must be outside that pane: inside it they would
				// scroll away with the fields, which is the defect.
				if view.save == nil || view.save.Hidden {
					t.Fatal("the save button is missing or hidden")
				}

				for _, tab := range []struct {
					index int
					name  string
				}{{1, "configuration"}, {0, "output"}} {
					view.panes.SelectIndex(tab.index)
					view.ui.window.Content().Resize(size)

					captured := view.ui.window.Canvas().Capture()
					if captured == nil {
						t.Fatalf("%s: the canvas captured nothing", tab.name)
					}
					if got := colourCount(captured); got < 4 {
						t.Errorf("%s: the render has %d colours — the widgets did not draw", tab.name, got)
					}

					if dir := os.Getenv("EZNIT_CAPTURE_DIR"); dir != "" {
						out := filepath.Join(dir, fmt.Sprintf("server-%s-%s-%.0fx%.0f.png",
							tab.name, name, size.Width, size.Height))
						if err := writePNG(out, captured); err != nil {
							t.Fatal(err)
						}
						t.Logf("captured %s", out)
					}
				}
			})
		}
	}
}

// TestServerTabKeepsTheSaveButtonVisible is the invariant the fix exists for:
// when the fields have to scroll, the save button is still drawn in the tab's
// bottom-right. It is a pixel check because Fyne has no absolute-position API:
// the brand primary is #2563EB and the High-importance button is the only thing
// that colour in that corner.
func TestServerTabKeepsTheSaveButtonVisible(t *testing.T) {
	dir := bundleDir(t, "API_PORT=\"4000\"\nDB_HOST=\"127.0.0.1\"\nDB_PORT=\"5432\"\nDB_USER=\"root\"\nDB_NAME=\"eznit\"\n")

	view := newTestServerView(t, dir)
	// Below the smallest supported layout on purpose: this is the one case the
	// fields cannot fit, so the pinned row is under pressure. A real window
	// refuses to shrink this far, because the pane's minimum is the form.
	size := fyne.NewSize(640, 360)
	view.ui.window.Resize(size)
	view.ui.window.Content().Resize(size)
	view.panes.SelectIndex(1)
	view.ui.window.Content().Resize(size)

	content, pane := view.configScroll.Content.MinSize(), view.configScroll.Size()
	if content.Height <= pane.Height {
		t.Skipf("the fields fit at %v, so the pinned row is not under pressure", size)
	}

	captured := view.ui.window.Canvas().Capture()
	if captured == nil {
		t.Fatal("the canvas captured nothing")
	}

	if dir := os.Getenv("EZNIT_CAPTURE_DIR"); dir != "" {
		_ = writePNG(filepath.Join(dir, "server-small-800x520.png"), captured)
	}

	// The High-importance button is filled with the theme's primary; nothing
	// else in that corner is.
	primary := view.ui.app.Settings().Theme().Color(theme.ColorNamePrimary, view.ui.app.Settings().ThemeVariant())
	if !hasColourInCorner(captured, primary) {
		t.Errorf("no save button in the bottom-right of a %v window: the action row scrolled with the fields", size)
	}
}

// hasColourInCorner reports whether the bottom-right corner of the image holds
// the given colour — where the save button is drawn.
func hasColourInCorner(img image.Image, want color.Color) bool {
	wr, wg, wb, _ := want.RGBA()

	bounds := img.Bounds()
	left := bounds.Max.X - bounds.Dx()/3
	top := bounds.Max.Y - 100

	for y := top; y < bounds.Max.Y; y++ {
		for x := left; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if near(uint8(r>>8), uint8(wr>>8)) && near(uint8(g>>8), uint8(wg>>8)) && near(uint8(b>>8), uint8(wb>>8)) {
				return true
			}
		}
	}

	return false
}

// near tolerates the antialiasing at a widget's edge.
func near(got, want uint8) bool {
	if got > want {
		return got-want <= 40
	}

	return want-got <= 40
}

// writePNG saves a capture so a reviewer can look at the layout a test proved.
func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

// colourCount is how many distinct colours an image holds: a render of widgets
// has several, a blank canvas has one.
func colourCount(img image.Image) int {
	seen := map[uint32]struct{}{}

	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 4 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 4 {
			r, g, b, a := img.At(x, y).RGBA()
			seen[r>>8<<24|g>>8<<16|b>>8<<8|a>>8] = struct{}{}
		}
	}

	return len(seen)
}
