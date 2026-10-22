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

// SafeRelPath validates a path that will be written inside a root folder and
// returns it in its canonical form: forward slashes, no leading or trailing
// separator.
//
// It refuses, rather than repairs, everything it cannot vouch for:
//
//   - an absolute path ("/etc/passwd", "C:\\Windows")
//   - a ".." segment, which is a traversal out of the root
//   - a "." segment and an empty segment, which mean the same thing as a
//     redundant separator
//   - a backslash, because a Windows host reads it as a separator: a path
//     that is safe on Linux is not safe on the machine that applies it
//   - a NUL byte, which truncates the path in the C library underneath
//
// The path is relative to a root the caller already trusts, so the root
// itself is not part of the check: `PathIsInside` answers that question.
//
// A template stores the path of a file it will later write into a user's
// project folder. The path comes from another user, so it is untrusted input
// that ends up in a file name: rejecting it here is the whole point.
func SafeRelPath(path string) (string, bool) {
	if path == "" {
		return "", false
	}

	if strings.ContainsRune(path, 0) {
		return "", false
	}

	// A backslash is a separator on Windows, so it is refused everywhere: the
	// path is stored once and applied on many machines.
	if strings.Contains(path, `\`) {
		return "", false
	}

	// An absolute path has no root to be relative to.
	if strings.HasPrefix(path, "/") {
		return "", false
	}

	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", false
		}
	}

	return strings.Join(segments, "/"), true
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
