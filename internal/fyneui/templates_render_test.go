//go:build fyne

package fyneui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"github.com/fiwon123/eznit/internal/domain/templates"
)

// TestTemplatesTabFitsItsWindow proves the catalogue and the detail pane both
// draw at the sizes a person gets, in both themes, without the window having to
// grow. The table's six fixed columns and the wrapped detail are the two things
// that could push it out.
func TestTemplatesTabFitsItsWindow(t *testing.T) {
	sizes := []fyne.Size{fyne.NewSize(980, 640), fyne.NewSize(800, 520)}
	themes := map[string]fyne.Theme{"light": theme.LightTheme(), "dark": theme.DarkTheme()}

	for name, th := range themes {
		for _, size := range sizes {
			t.Run(fmt.Sprintf("%s/%.0fx%.0f", name, size.Width, size.Height), func(t *testing.T) {
				view := newTestTemplatesView(t, apiChoice{url: "http://127.0.0.1:4000", source: "the default"})
				view.ui.app.Settings().SetTheme(th)

				detail := sampleDetail(t)
				view.items = []templates.TemplateData{detail.TemplateData}
				view.table.Refresh()
				view.showDetail(detail)
				view.updateActions()

				view.ui.window.Resize(size)
				view.ui.window.Content().Resize(size)

				content := view.ui.window.Content().MinSize()
				if content.Width > size.Width {
					t.Errorf("the tab needs %.0f px of width, the window has %.0f", content.Width, size.Width)
				}
				if content.Height > size.Height {
					t.Errorf("the tab needs %.0f px of height, the window has %.0f", content.Height, size.Height)
				}

				captured := view.ui.window.Canvas().Capture()
				if captured == nil {
					t.Fatal("the canvas captured nothing")
				}
				if got := colourCount(captured); got < 4 {
					t.Errorf("the render has %d colours — the widgets did not draw", got)
				}

				if dir := os.Getenv("EZNIT_CAPTURE_DIR"); dir != "" {
					out := filepath.Join(dir, fmt.Sprintf("templates-%s-%.0fx%.0f.png", name, size.Width, size.Height))
					if err := writePNG(out, captured); err != nil {
						t.Fatal(err)
					}
					t.Logf("captured %s", out)
				}
			})
		}
	}
}
