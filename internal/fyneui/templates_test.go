//go:build fyne

package fyneui

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/google/uuid"

	"github.com/fiwon123/eznit/internal/apiclient"
	"github.com/fiwon123/eznit/internal/config"
	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
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

	for _, want := range []string{"PROJECT", "Project name", "default demo", "required"} {
		if !strings.Contains(view.variables.Text, want) {
			t.Errorf("variables = %q, want it to mention %q", view.variables.Text, want)
		}
	}

	if braces := "{{"; strings.Contains(view.variables.Text, braces) {
		t.Errorf("variables = %q, want the keys without template braces", view.variables.Text)
	}

	if !strings.Contains(view.commands.Text, "scripts/run.sh") {
		t.Errorf("commands = %q, want the target path", view.commands.Text)
	}

	parts := map[string]*prose{
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

// Applying writes the template with the answers filled in, and leaves a binary
// file's bytes alone. This is the write path without the dialogs.
func TestApplyTemplateSubstitutesAndWrites(t *testing.T) {
	target := t.TempDir()

	detail := sampleDetail(t)
	detail.Files[0].Content = "# {{PROJECT}}\n"

	files, err := templateapply.Files(detail)
	if err != nil {
		t.Fatal(err)
	}

	if err := applyTemplate(target, files, map[string]string{"PROJECT": "eznit"}, false); err != nil {
		t.Fatalf("applyTemplate() = %v", err)
	}

	written, err := os.ReadFile(filepath.Join(target, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md was not written: %v", err)
	}
	if string(written) != "# eznit\n" {
		t.Errorf("AGENTS.md = %q, want the placeholder filled in", written)
	}

	binary, err := os.ReadFile(filepath.Join(target, "bin", "tool"))
	if err != nil {
		t.Fatalf("the binary file was not written: %v", err)
	}
	if !bytes.Equal(binary, []byte{0x7f, 'E', 'L', 'F'}) {
		t.Errorf("the binary file = %v, want its bytes untouched", binary)
	}

	if _, err := os.Stat(filepath.Join(target, "scripts", "run.sh")); err != nil {
		t.Errorf("the command was not written at its target path: %v", err)
	}
}

// A required answer that is empty stops the write; the other cases do not.
func TestMissingAnswer(t *testing.T) {
	variables := []templates.TemplateVariableData{
		{Key: "OPTIONAL"},
		{Key: "REQUIRED", Required: true},
		{Key: "SPACE", Required: true},
	}

	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"answered", map[string]string{"REQUIRED": "x", "SPACE": "y"}, ""},
		{"unanswered", map[string]string{"REQUIRED": "", "SPACE": "y"}, "REQUIRED"},
		{"whitespace", map[string]string{"REQUIRED": "x", "SPACE": "   "}, "SPACE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := missingAnswer(variables, tt.values); got != tt.want {
				t.Errorf("missingAnswer() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Apply without a selected template says so and opens nothing.
func TestApplyNeedsASelection(t *testing.T) {
	view := newTestTemplatesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	view.showDetail(nil)
	view.applySelected()

	if !strings.Contains(view.status.Text, "Select a template") {
		t.Errorf("status = %q, want it to ask for a selection", view.status.Text)
	}
}

// A template that asks nothing says so in words: a bare dash in the Variables
// section is not an answer anyone can act on.
func TestTemplatesEmptySectionsSaySo(t *testing.T) {
	view := newTestTemplatesView(t, apiChoice{url: "http://127.0.0.1:4321", source: "the default"})

	detail := sampleDetail(t)
	detail.Files = nil
	detail.Variables = nil
	detail.Commands = nil

	view.showDetail(detail)

	for name, section := range map[string]*prose{
		"files":     view.files,
		"variables": view.variables,
		"commands":  view.commands,
	} {
		if section.Text == "—" || section.Text == "" {
			t.Errorf("the %s section is %q, want a sentence that says it is empty", name, section.Text)
		}
	}

	if want := "asks nothing"; !strings.Contains(view.variables.Text, want) {
		t.Errorf("variables = %q, want %q", view.variables.Text, want)
	}
}

// The caption above an answer box names the question and the key, marks a
// required answer with the asterisk the hint explains, and never shows the
// template's braces.
func TestVariableCaption(t *testing.T) {
	tests := []struct {
		name     string
		variable templates.TemplateVariableData
		want     string
	}{
		{"prompt and key", templates.TemplateVariableData{Key: "PROJECT", Prompt: "Project name"}, "Project name (PROJECT)"},
		{"no prompt", templates.TemplateVariableData{Key: "PROJECT"}, "PROJECT"},
		{"prompt is the key", templates.TemplateVariableData{Key: "PROJECT", Prompt: "PROJECT"}, "PROJECT"},
		{"required", templates.TemplateVariableData{Key: "PROJECT", Prompt: "Project name", Required: true}, "Project name (PROJECT) *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := variableCaption(tt.variable); got != tt.want {
				t.Errorf("variableCaption(%+v) = %q, want %q", tt.variable, got, tt.want)
			}
			if strings.Contains(variableCaption(tt.variable), "{{") {
				t.Errorf("the caption shows template braces")
			}
		})
	}
}

// One variable line, in the order a person reads it: key, question, default,
// and whether it must be answered.
func TestVariableLine(t *testing.T) {
	tests := []struct {
		name     string
		variable templates.TemplateVariableData
		want     string
	}{
		{"everything", templates.TemplateVariableData{Key: "PROJECT", Prompt: "Project name", DefaultValue: "demo", Required: true}, "PROJECT — Project name — default demo — required"},
		{"key only", templates.TemplateVariableData{Key: "PROJECT"}, "PROJECT"},
		{"no default", templates.TemplateVariableData{Key: "PROJECT", Prompt: "Project name", Required: true}, "PROJECT — Project name — required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := variableLine(tt.variable); got != tt.want {
				t.Errorf("variableLine(%+v) = %q, want %q", tt.variable, got, tt.want)
			}
		})
	}
}
