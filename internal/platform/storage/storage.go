// Package storage is the single definition of where uploaded bytes live under
// the upload root.
//
// The files, templates, commands and account-deletion code each built these
// paths themselves, so a change in one place could disagree with another. They
// call this package instead.
//
// The layout:
//
//	<root>/<user-id>/<file-id>/<version>_<filename>   one version of an upload
//	<root>/templates/<template-id>/<file-id>/<name>   a binary template file
//	<root>/commands/<command-id>/<name>               a binary command
//
// Paths stored in the database (template and command files) are relative to the
// root and use forward slashes, so a row reads the same on every machine. Paths
// used on this machine are absolute and use the OS separator.
package storage

import (
	"path"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
)

// UserDir is the directory of everything one user uploaded.
func UserDir(root string, userID uuid.UUID) string {
	return filepath.Join(root, userID.String())
}

// FilePath is where one stored version of an upload lives on this machine:
// <root>/<user-id>/<file-id>/<version>_<filename>.
func FilePath(root string, userID, fileID uuid.UUID, version int, fileName string) string {
	return filepath.Join(UserDir(root, userID), fileID.String(), versionedName(version, fileName))
}

// versionedName is "<version>_<filename>": the on-disk name of one version, so
// two versions of the same file can sit in one folder without colliding.
func versionedName(version int, fileName string) string {
	return strconv.Itoa(version) + "_" + fileName
}

// TemplateDir is the directory of one template's binary files.
func TemplateDir(root string, templateID uuid.UUID) string {
	return filepath.Join(root, "templates", templateID.String())
}

// TemplateStoragePath is where a binary template file lives, relative to the
// root with forward slashes (the value is stored in a row and read on other
// machines). The file's own directory is its id, so two files with the same
// name in different folders cannot collide.
func TemplateStoragePath(templateID, fileID uuid.UUID, filePath string) string {
	return path.Join("templates", templateID.String(), fileID.String(), path.Base(filePath))
}

// CommandDir is the directory of one command's bytes.
func CommandDir(root string, commandID uuid.UUID) string {
	return filepath.Join(root, "commands", commandID.String())
}

// CommandStoragePath is where a binary command lives, relative to the root with
// forward slashes. It is built without path.Clean on purpose: the name is kept
// verbatim, as the row has always stored it, and the caller's PathIsInside
// check is what refuses an unsafe name.
func CommandStoragePath(commandID uuid.UUID, name string) string {
	return "commands/" + commandID.String() + "/" + name
}

// AbsolutePath resolves a stored (relative, forward-slash) path on this
// machine. The caller still checks it is inside the root with
// helper.PathIsInside.
func AbsolutePath(root, storagePath string) string {
	return filepath.Join(root, filepath.FromSlash(storagePath))
}
