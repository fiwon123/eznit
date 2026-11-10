//go:build fyne

package fyneui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

// logo.png is rendered from assets/logo.svg; the SVG is the source, the PNG is
// what the window manager and the About panel accept. Keep the two in step.
//
//go:embed logo.png
var logoPNG []byte

func logoResource() fyne.Resource {
	return fyne.NewStaticResource("eznit-logo.png", logoPNG)
}
