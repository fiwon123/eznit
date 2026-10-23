// Package web carries the built web UI inside the binaries.
//
// `make web_build` writes the UI to web/dist, and the embed directive below
// reads that folder at compile time, so a released binary serves the UI with
// no web/dist next to it. The directive cannot reach outside this package,
// which is why the file sits next to dist instead of in cmd/.
//
// web/dist is gitignored, so a fresh clone has no build output, and
// //go:embed refuses to compile against a folder that does not exist. The
// committed web/dist/.gitkeep keeps the folder valid for every build; callers
// ask HasIndex (see internal/platform/webui) rather than assuming a UI is
// there, so "placeholder only" means "no UI embedded", not a broken page.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded web UI, rooted at the dist folder.
//
// The result may hold only the committed placeholder: check for a UI before
// serving it.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		// Unreachable: "dist" is the path embedded above, so it always exists.
		panic("web: the embedded dist folder is missing: " + err.Error())
	}
	return sub
}
