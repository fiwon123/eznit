package templates

import (
	"bytes"
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func sampleDetail() *TemplateDetail {
	return &TemplateDetail{
		TemplateData: TemplateData{
			ID:          uuid.New(),
			Name:        "Starter",
			Slug:        "starter",
			Description: "a starter",
			Kind:        KindOpencode,
			Version:     2,
		},
		Files: []TemplateFileData{
			{
				ID:           uuid.New(),
				Path:         "AGENTS.md",
				Kind:         FileKindText,
				Content:      "# {{PROJECT_NAME}}",
				IsExecutable: false,
				Size:         16,
			},
			{
				ID:            uuid.New(),
				Path:          "logo.bin",
				Kind:          FileKindBinary,
				ContentBase64: base64.StdEncoding.EncodeToString([]byte{0x00, 0x01, 0xff}),
				Size:          3,
			},
		},
		Variables: []TemplateVariableData{
			{Key: "PROJECT_NAME", Prompt: "name?", Required: true, Position: 0},
		},
		Commands: []TemplateCommandData{
			{
				CommandID:    uuid.New(),
				TargetPath:   "bin/fmt",
				Name:         "fmt",
				Kind:         FileKindText,
				Content:      "#!/bin/sh\n",
				IsExecutable: true,
				Size:         10,
			},
		},
	}
}

func TestNewBundleFlattensCommands(t *testing.T) {
	bundle, err := NewBundle(sampleDetail())
	if err != nil {
		t.Fatalf("NewBundle() error = %v", err)
	}

	if bundle.Format != BundleFormat {
		t.Errorf("Format = %d, want %d", bundle.Format, BundleFormat)
	}
	if bundle.Template.Name != "Starter" || bundle.Template.Kind != KindOpencode {
		t.Errorf("template = %+v, want the detail's identity", bundle.Template)
	}

	// Two files plus the command at its target path.
	if len(bundle.Files) != 3 {
		t.Fatalf("files = %d, want 3", len(bundle.Files))
	}

	var command *BundleFile
	for i := range bundle.Files {
		if bundle.Files[i].Path == "bin/fmt" {
			command = &bundle.Files[i]
		}
	}
	if command == nil {
		t.Fatal("the command is missing from the bundle")
	}
	if command.Content != "#!/bin/sh\n" || !command.IsExecutable {
		t.Errorf("command = %+v, want its content and the executable bit", command)
	}
}

func TestBundleRoundTrip(t *testing.T) {
	detail := sampleDetail()
	want, err := NewBundle(detail)
	if err != nil {
		t.Fatalf("NewBundle() error = %v", err)
	}

	var buffer bytes.Buffer
	if err := WriteBundle(&buffer, want); err != nil {
		t.Fatalf("WriteBundle() error = %v", err)
	}

	got, err := ReadBundle(&buffer)
	if err != nil {
		t.Fatalf("ReadBundle() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	// The bytes of the binary file survive the trip.
	for _, file := range got.Files {
		if file.Path != "logo.bin" {
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(file.ContentBase64)
		if err != nil {
			t.Fatalf("binary file is not base64: %v", err)
		}
		if !bytes.Equal(decoded, []byte{0x00, 0x01, 0xff}) {
			t.Errorf("binary bytes = %v, want the original", decoded)
		}
	}
}

// A hand-written bundle may leave the kinds out; normalise fills the same
// defaults the publish path uses.
func TestReadBundleFillsDefaults(t *testing.T) {
	raw := `{
	  "format": 1,
	  "template": {"name": "minimal"},
	  "files": [{"path": "README.md", "content": "hi"}]
	}`

	bundle, err := ReadBundle(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadBundle() error = %v", err)
	}

	if bundle.Template.Kind != KindGeneric {
		t.Errorf("template kind = %q, want %q", bundle.Template.Kind, KindGeneric)
	}
	if bundle.Files[0].Kind != FileKindText {
		t.Errorf("file kind = %q, want %q", bundle.Files[0].Kind, FileKindText)
	}
}

func TestReadBundleRefusesUnknownFields(t *testing.T) {
	raw := `{
	  "format": 1,
	  "template": {"name": "typo"},
	  "files": [{"path": "README.md", "content": "hi"}],
	  "variable": []
	}`

	if _, err := ReadBundle(strings.NewReader(raw)); err == nil {
		t.Fatal("ReadBundle() error = nil, want the unknown field refused")
	}
}

func TestBundleValidate(t *testing.T) {
	valid := func() *Bundle {
		bundle, err := NewBundle(sampleDetail())
		if err != nil {
			t.Fatalf("NewBundle() error = %v", err)
		}
		return bundle
	}

	largeText := strings.Repeat("a", maxTextFileBytes+1)
	largeBinary := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, maxBinaryFileBytes+1))
	largePrompt := strings.Repeat("p", maxPromptLength+1)
	longName := strings.Repeat("n", maxNameLength+1)

	tests := []struct {
		name    string
		change  func(*Bundle)
		wantErr bool
	}{
		{"valid", func(*Bundle) {}, false},
		{"unsupported format", func(b *Bundle) { b.Format = 99 }, true},
		{"no name", func(b *Bundle) { b.Template.Name = "  " }, true},
		{"long name", func(b *Bundle) { b.Template.Name = longName }, true},
		{"unknown kind", func(b *Bundle) { b.Template.Kind = "vim" }, true},
		{"no files", func(b *Bundle) { b.Files = nil }, true},
		{"unsafe parent path", func(b *Bundle) { b.Files[0].Path = "../escape" }, true},
		{"absolute path", func(b *Bundle) { b.Files[0].Path = "/etc/passwd" }, true},
		{"backslash path", func(b *Bundle) { b.Files[0].Path = "dir\\file" }, true},
		{"duplicate path", func(b *Bundle) { b.Files[1].Path = b.Files[0].Path }, true},
		{"unknown file kind", func(b *Bundle) { b.Files[0].Kind = "huge" }, true},
		{"binary with text", func(b *Bundle) { b.Files[1].Content = "oops" }, true},
		{"binary without bytes", func(b *Bundle) { b.Files[1].ContentBase64 = "" }, true},
		{"binary bad base64", func(b *Bundle) { b.Files[1].ContentBase64 = "not base64!" }, true},
		{"text with base64", func(b *Bundle) { b.Files[0].ContentBase64 = "aGk=" }, true},
		{"text too large", func(b *Bundle) { b.Files[0].Content = largeText }, true},
		{"binary too large", func(b *Bundle) { b.Files[1].ContentBase64 = largeBinary }, true},
		{"bad variable name", func(b *Bundle) { b.Variables[0].Key = "project" }, true},
		{"duplicate variable", func(b *Bundle) {
			b.Variables = append(b.Variables, b.Variables[0])
		}, true},
		{"long prompt", func(b *Bundle) { b.Variables[0].Prompt = largePrompt }, true},
		{"negative position", func(b *Bundle) { b.Variables[0].Position = -1 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := valid()
			tt.change(bundle)

			err := bundle.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriteBundleRefusesInvalid(t *testing.T) {
	bundle, err := NewBundle(sampleDetail())
	if err != nil {
		t.Fatalf("NewBundle() error = %v", err)
	}
	bundle.Files[0].Path = "../escape"

	var buffer bytes.Buffer
	if err := WriteBundle(&buffer, bundle); err == nil {
		t.Fatal("WriteBundle() error = nil, want the unsafe path refused")
	}
	if buffer.Len() != 0 {
		t.Errorf("WriteBundle() wrote %d bytes for an invalid bundle, want 0", buffer.Len())
	}
}
