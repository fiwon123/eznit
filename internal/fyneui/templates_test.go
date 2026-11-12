//go:build fyne

package fyneui

import (
	"bytes"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/google/uuid"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/domain/templates"
)

// newTestTemplatesView builds the tab for an address. Nothing is fetched: the
// tests put their own rows in, which is the only way to exercise the rendering
// without a server.
func newTestTemplatesView(t *testing.T, address apiChoice) *templatesView {
	t.Helper()

	t.Setenv(config.Env, filepath.Join(t.TempDir(), "config.json"))

	app := test.NewApp()
	t.Cleanup(app.Quit)

	u := &ui{
		app:     app,
		window:  app.NewWindow("test"),
		address: address,
		client:  apiclient.New(address.url),
	}

	u.templates = newTemplatesView(u)
	u.window.SetContent(u.templates.content())

	return u.templates
}

// sampleDetail is one template with every part a section can show: two files
// (one of them binary and executable), a variable and a command.
func sampleDetail(t *testing.T) *templates.TemplateDetail {
	t.Helper()

	return &templates.TemplateDetail{
		TemplateData: templates.TemplateData{
			ID:          uuid.MustParse("6f1c1f18-5c2e-4b1e-9d0e-6a1f3b3c0a11"),
			Name:        "My starter",
			Slug:        "my-starter",
			Description: "The files a new project starts with.",
			Kind:        templates.KindGeneric,
			Version:     3,
			IsPublic:    true,
			OwnerEmail:  "you@example.com",
			FileCount:   2,
		},
		Files: []templates.TemplateFileData{
			{ID: uuid.New(), Path: "AGENTS.md", Kind: templates.FileKindText, Size: 2048, Content: "# hi"},
			{ID: uuid.New(), Path: "bin/tool", Kind: templates.FileKindBinary, Size: 4, ContentBase64: base64.StdEncoding.EncodeToString([]byte{0x7f, 'E', 'L', 'F'}), IsExecutable: true},
		},
		Variables: []templates.TemplateVariableData{
			{ID: uuid.New(), Key: "PROJECT", Prompt: "Project name", DefaultValue: "demo", Required: true},
		},
		Commands: []templates.TemplateCommandData{
			{CommandID: uuid.New(), TargetPath: "scripts/run.sh", Name: "run", Kind: templates.FileKindText, Size: 12, Content: "#!/bin/sh\n"},
		},
	}
}

// The detail panel is the whole template in four selectable parts.
func TestTemplatesDetailShowsEveryPart(t *testing.T) {
	view := newTestTemplatesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.showDetail(sampleDetail(t))
	view.updateActions()

	for _, want := range []string{"My starter", "version 3", "shared", "you@example.com", "The files a new project starts with."} {
		if !strings.Contains(view.summary.Text, want) {
			t.Errorf("summary = %q, want it to mention %q", view.summary.Text, want)
		}
	}

	for _, want := range []string{"AGENTS.md", "2.0 KiB", "bin/tool", "executable"} {
		if !strings.Contains(view.files.Text, want) {
			t.Errorf("files = %q, want it to mention %q", view.files.Text, want)
		}
	}

	for _, want := range []string{"{{PROJECT}}", "Project name", "default demo", "required"} {
		if !strings.Contains(view.variables.Text, want) {
			t.Errorf("variables = %q, want it to mention %q", view.variables.Text, want)
		}
	}

	if !strings.Contains(view.commands.Text, "scripts/run.sh") {
		t.Errorf("commands = %q, want the target path", view.commands.Text)
	}

	parts := map[string]*widget.Label{
		"summary":   view.summary,
		"files":     view.files,
		"variables": view.variables,
		"commands":  view.commands,
	}
	for name, label := range parts {
		if !label.Selectable {
			t.Errorf("%s is not selectable, so it cannot be copied", name)
		}
	}

	if view.export.Disabled() {
		t.Error("Save as bundle is disabled although a template is loaded")
	}
}

// Without a selection the panel says what to do and the export is off.
func TestTemplatesWithoutASelection(t *testing.T) {
	view := newTestTemplatesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.showDetail(nil)
	view.updateActions()

	if !strings.Contains(view.summary.Text, "Select a template") {
		t.Errorf("summary = %q, want it to ask for a selection", view.summary.Text)
	}
	if !view.export.Disabled() {
		t.Error("Save as bundle is enabled with no template loaded")
	}
}

// The bundle is the CLI's format: writing one and reading it back must give the
// same files, the command flattened to its target path, and the executable bit.
func TestExportBundleRoundTrips(t *testing.T) {
	var written bytes.Buffer

	if err := exportBundle(&written, sampleDetail(t)); err != nil {
		t.Fatalf("exportBundle() = %v", err)
	}

	bundle, err := templates.ReadBundle(bytes.NewReader(written.Bytes()))
	if err != nil {
		t.Fatalf("the bundle this tab writes is not one ReadBundle accepts: %v", err)
	}

	if bundle.Template.Name != "My starter" || bundle.Template.Slug != "my-starter" {
		t.Errorf("template = %+v, want the identity", bundle.Template)
	}

	byPath := map[string]templates.BundleFile{}
	for _, file := range bundle.Files {
		byPath[file.Path] = file
	}

	if len(bundle.Files) != 3 {
		t.Fatalf("the bundle holds %d files, want the two files and the command: %v", len(bundle.Files), byPath)
	}

	if got := byPath["AGENTS.md"].Content; got != "# hi" {
		t.Errorf("AGENTS.md content = %q", got)
	}

	binary, ok := byPath["bin/tool"]
	if !ok {
		t.Fatal("the binary file is missing from the bundle")
	}
	decoded, err := base64.StdEncoding.DecodeString(binary.ContentBase64)
	if err != nil {
		t.Fatalf("the binary file is not base64: %v", err)
	}
	if !bytes.Equal(decoded, []byte{0x7f, 'E', 'L', 'F'}) || !binary.IsExecutable {
		t.Errorf("binary = %+v, want the original bytes and the executable bit", binary)
	}

	if _, ok := byPath["scripts/run.sh"]; !ok {
		t.Error("the command was not flattened to its target path")
	}

	if len(bundle.Variables) != 1 || bundle.Variables[0].Key != "PROJECT" || !bundle.Variables[0].Required {
		t.Errorf("variables = %+v, want the required PROJECT", bundle.Variables)
	}
}

// The suggested name is a filename, not a title: no spaces, no punctuation the
// filesystem or a shell would mind.
func TestBundleFilename(t *testing.T) {
	tests := []struct {
		name  string // the case
		title string // the template's name
		slug  string
		want  string
	}{
		{"a slug wins", "Ignored", "my-starter", "my-starter.eznit"},
		{"spaces become dashes", "My Starter", "", "my-starter.eznit"},
		{"punctuation goes", "A / b? c!", "", "a-b-c.eznit"},
		{"nothing usable", "???", "", "template.eznit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail := &templates.TemplateDetail{TemplateData: templates.TemplateData{Slug: tt.slug, Name: tt.title}}

			if got := bundleFilename(detail); got != tt.want {
				t.Errorf("bundleFilename(%q, %q) = %q, want %q", tt.slug, tt.title, got, tt.want)
			}
		})
	}
}

// Deselecting drops the detail too: Save as bundle must not stay on for a
// template that is no longer selected.
func TestTemplatesHeaderRowDeselects(t *testing.T) {
	view := newTestTemplatesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	detail := sampleDetail(t)
	view.items = []templates.TemplateData{detail.TemplateData}
	view.showDetail(detail)
	view.updateActions()

	if view.export.Disabled() {
		t.Fatal("Save as bundle is off before anything was deselected")
	}

	view.table.OnSelected(widget.TableCellID{Row: 0})

	if !view.export.Disabled() {
		t.Error("Save as bundle is still on after the selection was cleared")
	}
	if !strings.Contains(view.summary.Text, "Select a template") {
		t.Errorf("summary = %q, want the empty state", view.summary.Text)
	}
}
