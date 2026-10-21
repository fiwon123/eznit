package helper

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A helper function to create a new folder if not exists
func CreatePathIfNotExists(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}

	return nil
}

// A helper function to verify if path exists
func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// PathIsInside reports whether path is inside the root directory. It is the
// check that turns "the path was built from a safe name" into "the path is
// inside the root": a prefix comparison is not enough, because a root of
// /data/uploads also prefixes /data/uploads-other, so the two are compared
// as a relative path instead.
func PathIsInside(root string, path string) bool {
	if root == "" || path == "" {
		return false
	}

	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return true
}
