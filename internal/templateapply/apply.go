// Package templateapply writes a template into a folder: the paths it may
// write, the placeholders it fills in, and the files themselves.
//
// It is here rather than in the CLI because two callers need it — `template
// apply` and the desktop window. Safe paths, no silent overwrite, and base64
// decoded before anything touches disk are one implementation, so the two
// cannot disagree about what applying means.
package templateapply

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/helper"
)

// File is one file of a template on the client side: the path it takes inside
// the project, and its content.
//
// Kind is FileKindText or FileKindBinary; an empty Kind is text, so a starter
// template and an old API response read the same. A binary file carries Bytes,
// decoded once, as well as the base64 it arrived as.
type File struct {
	Path          string
	Kind          string
	Content       string
	ContentBase64 string
	Bytes         []byte
	IsExecutable  bool
}

// placeholderPattern matches a variable placeholder: {{PROJECT_NAME}}. The
// name has the shape the API accepts, so a placeholder found here is one the
// server would accept, and one that is not in this shape is left alone — the
// template may use braces for something else.
var placeholderPattern = regexp.MustCompile(`\{\{([A-Z][A-Z0-9_]{0,63})\}\}`)

// Files turns a template into the files to write: its own files, then the
// commands it pulls in at their target paths.
//
// A binary file is decoded here, once, before anything is written: a body that
// does not decode is a failure, not a file full of rubbish on disk.
func Files(detail *templates.TemplateDetail) ([]File, error) {
	if detail == nil {
		return nil, fmt.Errorf("there is no template to apply")
	}

	files := make([]File, 0, len(detail.Files)+len(detail.Commands))

	for _, file := range detail.Files {
		entry, err := FileOf(file.Path, file.Kind, file.Content, file.ContentBase64, file.IsExecutable)
		if err != nil {
			return nil, err
		}

		files = append(files, entry)
	}

	// A command a template pulls in is written at its target path, the same way
	// a file is: the bytes come from the library through the detail.
	for _, command := range detail.Commands {
		entry, err := FileOf(command.TargetPath, command.Kind, command.Content, command.ContentBase64, command.IsExecutable)
		if err != nil {
			return nil, fmt.Errorf("the command %q: %w", command.TargetPath, err)
		}

		files = append(files, entry)
	}

	return files, nil
}

// FileOf builds one writeable file, decoding a binary one. A caller whose files
// arrive in another shape — a bundle's, say — uses this instead of decoding a
// second time.
func FileOf(path, kind, content, contentBase64 string, executable bool) (File, error) {
	entry := File{
		Path:         path,
		Kind:         kind,
		Content:      content,
		IsExecutable: executable,
	}

	if kind != templates.FileKindBinary {
		return entry, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return File{}, fmt.Errorf("the file %q is not valid base64: %w", path, err)
	}

	entry.Bytes = decoded
	entry.ContentBase64 = contentBase64

	return entry, nil
}

// Substitute replaces the placeholders that have a value. A placeholder with no
// value is left as it is.
func Substitute(content string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(content, func(match string) string {
		key := match[2 : len(match)-2]

		if value, ok := values[key]; ok {
			return value
		}

		return match
	})
}

// Undeclared returns the placeholders the files hold that the template does not
// declare, sorted and unique. With no variables it lists every placeholder —
// which is what publishing a folder needs.
func Undeclared(files []File, variables []templates.TemplateVariableData) []string {
	declared := make(map[string]bool, len(variables))
	for _, variable := range variables {
		declared[variable.Key] = true
	}

	seen := map[string]bool{}
	var keys []string

	for _, file := range files {
		for _, match := range placeholderPattern.FindAllStringSubmatch(file.Content, -1) {
			key := match[1]
			if declared[key] || seen[key] {
				continue
			}

			seen[key] = true
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)

	return keys
}

// Existing lists the files a write into target would overwrite, sorted. It
// also refuses the files a write must never attempt: two entries at one path,
// and a path that would land outside the folder — the paths come from the
// server, and the server got them from another user.
func Existing(target string, files []File) ([]string, error) {
	var existing []string

	seen := make(map[string]bool, len(files))

	for _, file := range files {
		if seen[file.Path] {
			return nil, fmt.Errorf("the template writes %q twice", file.Path)
		}

		seen[file.Path] = true

		path := filepath.Join(target, filepath.FromSlash(file.Path))
		if !helper.PathIsInside(target, path) {
			return nil, fmt.Errorf("the path %q would land outside the folder", file.Path)
		}

		found, err := helper.PathExists(path)
		if err != nil {
			return nil, err
		}

		if found {
			existing = append(existing, file.Path)
		}
	}

	sort.Strings(existing)

	return existing, nil
}

// Check refuses a write that must not happen, with the message the CLI prints.
// It runs before the questions are asked: a user is not asked what a template
// wants to know when the template is not going to be written.
func Check(target string, files []File, overwrite bool) error {
	existing, err := Existing(target, files)
	if err != nil {
		return err
	}

	if overwrite || len(existing) == 0 {
		return nil
	}

	return fmt.Errorf("these files are already there: %s (use --force to overwrite them)", strings.Join(existing, ", "))
}

// Write writes the files of a template into a folder.
//
// Every path is checked before any file is written: a tree that is half
// written is worse than a tree that is not written at all.
func Write(target string, files []File, overwrite bool) error {
	if err := Check(target, files, overwrite); err != nil {
		return err
	}

	for _, file := range files {
		path := filepath.Join(target, filepath.FromSlash(file.Path))

		if err := helper.CreatePathIfNotExists(filepath.Dir(path)); err != nil {
			return err
		}

		mode := os.FileMode(0644)
		if file.IsExecutable {
			mode = 0755
		}

		content := []byte(file.Content)
		if file.Kind == templates.FileKindBinary {
			content = file.Bytes
		}

		if err := os.WriteFile(path, content, mode); err != nil {
			return err
		}

		// A file that was already there keeps its own mode, so the executable
		// flag is set on its own.
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}

	return nil
}
