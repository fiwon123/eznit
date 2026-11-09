package templates

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/fiwon123/eznit/pkg/helper"
)

// BundleFormat is the version of the offline bundle. A file carries it so a
// future change can be detected instead of misread.
const BundleFormat = 1

// Bundle is a whole template in one JSON document: what `template export`
// writes and `template apply --from-file` reads. It needs no server and no
// account, which is the point: a template is files, and files can travel.
//
// The files are flattened: a command a template pulls in is written at its
// target path, exactly as apply writes it. A bundle is a snapshot to apply,
// not a publish request to send back.
type Bundle struct {
	Format    int              `json:"format"`
	Template  BundleTemplate   `json:"template"`
	Variables []BundleVariable `json:"variables,omitempty"`
	Files     []BundleFile     `json:"files"`
}

// BundleTemplate is the identity of the template inside a bundle.
type BundleTemplate struct {
	Name        string `json:"name"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Version     int    `json:"version,omitempty"`
}

// BundleVariable is one question the template asks when it is applied. The
// prompt and the default are in the file; the answer is asked locally and
// never stored.
type BundleVariable struct {
	Key          string `json:"key"`
	Prompt       string `json:"prompt,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	Required     bool   `json:"required,omitempty"`
	Position     int    `json:"position,omitempty"`
}

// BundleFile is one file of the bundle. A text file carries Content; a binary
// file carries ContentBase64, because arbitrary bytes do not survive JSON.
type BundleFile struct {
	Path          string `json:"path"`
	Kind          string `json:"kind,omitempty"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
	IsExecutable  bool   `json:"is_executable,omitempty"`
	Size          int64  `json:"size,omitempty"`
}

// NewBundle turns a template into a bundle, flattening its command references
// into files at their target paths.
func NewBundle(detail *TemplateDetail) (*Bundle, error) {
	if detail == nil {
		return nil, errors.New("a bundle needs a template")
	}

	bundle := &Bundle{
		Format: BundleFormat,
		Template: BundleTemplate{
			Name:        detail.Name,
			Slug:        detail.Slug,
			Description: detail.Description,
			Kind:        detail.Kind,
			Version:     detail.Version,
		},
		Files: make([]BundleFile, 0, len(detail.Files)+len(detail.Commands)),
	}

	for _, file := range detail.Files {
		bundle.Files = append(bundle.Files, BundleFile{
			Path:          file.Path,
			Kind:          file.Kind,
			Content:       file.Content,
			ContentBase64: file.ContentBase64,
			IsExecutable:  file.IsExecutable,
			Size:          file.Size,
		})
	}

	for _, command := range detail.Commands {
		bundle.Files = append(bundle.Files, BundleFile{
			Path:          command.TargetPath,
			Kind:          command.Kind,
			Content:       command.Content,
			ContentBase64: command.ContentBase64,
			IsExecutable:  command.IsExecutable,
			Size:          command.Size,
		})
	}

	for _, variable := range detail.Variables {
		bundle.Variables = append(bundle.Variables, BundleVariable{
			Key:          variable.Key,
			Prompt:       variable.Prompt,
			DefaultValue: variable.DefaultValue,
			Required:     variable.Required,
			Position:     variable.Position,
		})
	}

	bundle.normalise()

	if err := bundle.Validate(); err != nil {
		return nil, err
	}

	return bundle, nil
}

// WriteBundle writes the bundle as indented JSON. It validates first, so a
// file that would be refused on apply is never written.
func WriteBundle(w io.Writer, bundle *Bundle) error {
	if bundle == nil {
		return errors.New("no bundle to write")
	}

	bundle.normalise()

	if err := bundle.Validate(); err != nil {
		return err
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(bundle); err != nil {
		return fmt.Errorf("write the bundle: %w", err)
	}

	return nil
}

// ReadBundle reads and validates a bundle. Unknown fields are refused: a
// bundle is machine-written, and a hand edit with a typo should fail loudly
// rather than silently lose the field.
func ReadBundle(r io.Reader) (*Bundle, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var bundle Bundle
	if err := decoder.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("read the bundle: %w", err)
	}

	bundle.normalise()

	if err := bundle.Validate(); err != nil {
		return nil, err
	}

	return &bundle, nil
}

// normalise fills the defaults the rest of the file relies on: an empty kind
// is generic, an empty file kind is text. It is what a bundle written by hand
// gets as well as one written by an older client.
func (b *Bundle) normalise() {
	if strings.TrimSpace(b.Template.Kind) == "" {
		b.Template.Kind = KindGeneric
	}

	for i := range b.Files {
		if strings.TrimSpace(b.Files[i].Kind) == "" {
			b.Files[i].Kind = FileKindText
		}
	}
}

// Validate applies the same rules the server applies to a publish: safe
// relative paths, unique paths, text or binary but not both, and the size
// caps. A bundle must not be a way around them.
func (b *Bundle) Validate() error {
	if b.Format != BundleFormat {
		return fmt.Errorf("bundle format %d is not supported (this build reads format %d)", b.Format, BundleFormat)
	}

	name := strings.TrimSpace(b.Template.Name)
	if name == "" {
		return errors.New("the bundle names no template")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("the template name is longer than %d characters", maxNameLength)
	}
	if len(b.Template.Description) > maxDescription {
		return fmt.Errorf("the template description is longer than %d characters", maxDescription)
	}
	if !IsKind(b.Template.Kind) {
		return fmt.Errorf("%q is not a template kind this build knows", b.Template.Kind)
	}

	if len(b.Files) == 0 {
		return errors.New("the bundle holds no file")
	}
	if len(b.Files) > maxFilesPerTemplate {
		return fmt.Errorf("the bundle holds %d files; a template allows %d", len(b.Files), maxFilesPerTemplate)
	}
	if len(b.Variables) > maxVariables {
		return fmt.Errorf("the bundle asks %d variables; a template allows %d", len(b.Variables), maxVariables)
	}

	seenPaths := make(map[string]bool, len(b.Files))
	var textTotal, binaryTotal int

	for _, file := range b.Files {
		path, ok := helper.SafeRelPath(strings.TrimSpace(file.Path))
		if !ok {
			return fmt.Errorf("the file path %q cannot be written safely", file.Path)
		}
		if seenPaths[path] {
			return fmt.Errorf("the bundle holds %q twice", path)
		}
		seenPaths[path] = true

		if !IsFileKind(file.Kind) {
			return fmt.Errorf("the file %q has an unknown kind %q", path, file.Kind)
		}

		if file.Kind == FileKindBinary {
			if file.Content != "" {
				return fmt.Errorf("the binary file %q carries content; use content_base64", path)
			}
			if file.ContentBase64 == "" {
				return fmt.Errorf("the binary file %q carries no content_base64", path)
			}

			decoded, err := base64.StdEncoding.DecodeString(file.ContentBase64)
			if err != nil {
				return fmt.Errorf("the binary file %q is not valid base64: %w", path, err)
			}

			if len(decoded) > maxBinaryFileBytes {
				return fmt.Errorf("the binary file %q is larger than %d bytes", path, maxBinaryFileBytes)
			}

			binaryTotal += len(decoded)
			if binaryTotal > maxBinaryTemplateBytes {
				return fmt.Errorf("the binary files are larger than %d bytes together", maxBinaryTemplateBytes)
			}

			continue
		}

		if file.ContentBase64 != "" {
			return fmt.Errorf("the text file %q carries content_base64; use content", path)
		}
		if len(file.Content) > maxTextFileBytes {
			return fmt.Errorf("the text file %q is larger than %d bytes", path, maxTextFileBytes)
		}

		textTotal += len(file.Content)
		if textTotal > maxTextTemplateBytes {
			return fmt.Errorf("the text files are larger than %d bytes together", maxTextTemplateBytes)
		}
	}

	seenKeys := make(map[string]bool, len(b.Variables))
	for _, variable := range b.Variables {
		if !variableKeyPattern.MatchString(variable.Key) {
			return fmt.Errorf("%q is not a variable name (upper case letters, digits and _, starting with a letter)", variable.Key)
		}
		if seenKeys[variable.Key] {
			return fmt.Errorf("the bundle declares {{%s}} twice", variable.Key)
		}
		seenKeys[variable.Key] = true

		if len(variable.Prompt) > maxPromptLength {
			return fmt.Errorf("the prompt for {{%s}} is longer than %d characters", variable.Key, maxPromptLength)
		}
		if variable.Position < 0 {
			return fmt.Errorf("the position of {{%s}} is negative", variable.Key)
		}
	}

	return nil
}
