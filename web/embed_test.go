package web

import (
	"io/fs"
	"testing"
)

// FS() is rooted at the dist folder: "dist" is not part of a path inside it.
// Without the fs.Sub in FS(), every caller would have to prefix "dist/" to
// look a file up, and a request for /index.html would miss.
func TestFSIsRootedAtDist(t *testing.T) {
	fsys := FS()

	if _, err := fs.Stat(fsys, "."); err != nil {
		t.Fatalf("the embedded file system cannot be read: %v", err)
	}

	if _, err := fs.Stat(fsys, "dist"); err == nil {
		t.Error(`FS() exposes "dist" as a subfolder; it should be the root`)
	}
}
