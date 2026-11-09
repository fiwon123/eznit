package cli

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/google/uuid"
)

// Template Apply Cmd applies a template to a folder: the files are written
// and the variables the template asks for are filled in.
//
// The template comes from the API (an id, or a choice from the catalogue) or
// from a bundle written by `template export --from-file`, which needs no
// server at all.
type TemplateApplyCmd struct {
	ID       string `arg:"" optional:"" help:"the id of the template"`
	FromFile string `help:"apply a bundle written by template export, instead of a template id"`
	Target   string `help:"the folder to write the template into" default:"."`
	Force    bool   `help:"overwrite the files that are already there"`
}

// source is what an apply writes: the files, the questions, and the
// placeholders the template forgot to declare — from the API or from a file.
type source struct {
	Name       string
	Files      []templateFile
	Variables  []templates.TemplateVariableData
	Undeclared []string
}

// Run applies the template.
func (cmd *TemplateApplyCmd) Run(g *Globals) error {
	if cmd.FromFile != "" && strings.TrimSpace(cmd.ID) != "" {
		g.logger.Warn("give a template id or --from-file, not both. ")
		return nil
	}

	src, ok := cmd.load(g)
	if !ok {
		return nil
	}

	target := cmd.Target
	if target == "" {
		target = "."
	}

	// The files are checked before the questions are asked: a user is not
	// asked what a template wants to know when the template is not going to
	// be written.
	if err := checkTemplateFiles(target, src.Files, cmd.Force); err != nil {
		g.logger.Warn("the template was not applied. ", slog.String("error", err.Error()))
		return nil
	}

	// The values are asked for here, on the machine that writes the files:
	// a value the user types never travels to the server, and a bundle
	// carries no answers at all.
	values, ok := askTemplateValues(src.Variables)
	if !ok {
		return nil
	}

	// A binary file has no placeholders: its bytes are what they are.
	for i := range src.Files {
		if src.Files[i].Kind != templates.FileKindBinary {
			src.Files[i].Content = substitute(src.Files[i].Content, values)
		}
	}

	// A placeholder the template does not declare is left alone: it may be
	// something else entirely. It is named, so the user can see it.
	for _, key := range src.Undeclared {
		fmt.Printf("note: {{%s}} is in the files but the template does not declare it\n", key)
	}

	if err := writeTemplateFiles(target, src.Files, cmd.Force); err != nil {
		g.logger.Warn("the template was not applied. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Println()
	fmt.Printf("applied %s to %s (%d files)\n", src.Name, target, len(src.Files))
	fmt.Println()

	return nil
}

// load reads the source: a bundle from disk, or a template through the API.
func (cmd *TemplateApplyCmd) load(g *Globals) (*source, bool) {
	if cmd.FromFile != "" {
		return bundleSource(g, cmd.FromFile)
	}

	// Applying a public template writes files on this machine and asks the
	// server nothing else, so it needs no account. A session only widens
	// what can be read — the caller's own private templates.
	token, _ := getToken()

	detail, ok := findTemplate(g, cmd.ID, token)
	if !ok {
		return nil, false
	}

	files, err := templateFilesOf(detail)
	if err != nil {
		g.logger.Warn("the template was not applied. ", slog.String("error", err.Error()))
		return nil, false
	}

	return &source{
		Name:       detail.Name,
		Files:      files,
		Variables:  detail.Variables,
		Undeclared: undeclaredPlaceholders(files, detail.Variables),
	}, true
}

// bundleSource reads a bundle from a file, with no server and no session.
func bundleSource(g *Globals, path string) (*source, bool) {
	file, err := os.Open(path)
	if err != nil {
		g.logger.Warn("the bundle could not be opened. ", slog.String("error", err.Error()))
		return nil, false
	}
	defer file.Close()

	bundle, err := templates.ReadBundle(file)
	if err != nil {
		g.logger.Warn("the bundle was not read. ", slog.String("error", err.Error()))
		return nil, false
	}

	files, err := filesFromBundle(bundle)
	if err != nil {
		g.logger.Warn("the bundle was not applied. ", slog.String("error", err.Error()))
		return nil, false
	}

	variables := make([]templates.TemplateVariableData, 0, len(bundle.Variables))
	for _, variable := range bundle.Variables {
		variables = append(variables, templates.TemplateVariableData{
			Key:          variable.Key,
			Prompt:       variable.Prompt,
			DefaultValue: variable.DefaultValue,
			Required:     variable.Required,
			Position:     variable.Position,
		})
	}

	return &source{
		Name:       bundle.Template.Name,
		Files:      files,
		Variables:  variables,
		Undeclared: undeclaredPlaceholders(files, variables),
	}, true
}

// filesFromBundle turns the files of a bundle into the files the client
// writes. The bundle was validated when it was read; the base64 decode here is
// where a binary file becomes bytes on disk.
func filesFromBundle(bundle *templates.Bundle) ([]templateFile, error) {
	files := make([]templateFile, 0, len(bundle.Files))

	for _, file := range bundle.Files {
		entry := templateFile{
			Path:         file.Path,
			Kind:         file.Kind,
			Content:      file.Content,
			IsExecutable: file.IsExecutable,
		}

		if file.Kind == templates.FileKindBinary {
			bytes, err := base64.StdEncoding.DecodeString(file.ContentBase64)
			if err != nil {
				return nil, fmt.Errorf("the file %q is not valid base64: %w", file.Path, err)
			}

			entry.Bytes = bytes
			entry.ContentBase64 = file.ContentBase64
		}

		files = append(files, entry)
	}

	return files, nil
}

// templateFilesOf turns the files of a template into the files the client
// writes: the same paths, and the contents as they came.
//
// A binary file arrives as base64 and is decoded here, once, before anything
// is written: a body that does not decode is a failure, not a file full of
// rubbish on disk.
func templateFilesOf(detail *templates.TemplateDetail) ([]templateFile, error) {
	files := make([]templateFile, 0, len(detail.Files))

	for _, file := range detail.Files {
		entry := templateFile{
			Path:         file.Path,
			Kind:         file.Kind,
			Content:      file.Content,
			IsExecutable: file.IsExecutable,
		}

		if file.Kind == templates.FileKindBinary {
			bytes, err := base64.StdEncoding.DecodeString(file.ContentBase64)
			if err != nil {
				return nil, fmt.Errorf("the file %q is not valid base64: %w", file.Path, err)
			}

			entry.Bytes = bytes
			entry.ContentBase64 = file.ContentBase64
		}

		files = append(files, entry)
	}

	// A command a template pulls in is written at its target path, the same
	// way a file is: the bytes come from the library through the detail.
	for _, command := range detail.Commands {
		entry := templateFile{
			Path:         command.TargetPath,
			Kind:         command.Kind,
			Content:      command.Content,
			IsExecutable: command.IsExecutable,
		}

		if command.Kind == templates.FileKindBinary {
			bytes, err := base64.StdEncoding.DecodeString(command.ContentBase64)
			if err != nil {
				return nil, fmt.Errorf("the command %q is not valid base64: %w", command.TargetPath, err)
			}

			entry.Bytes = bytes
			entry.ContentBase64 = command.ContentBase64
		}

		files = append(files, entry)
	}

	return files, nil
}

// findTemplate resolves the template to use: the one named, or one picked from
// the catalogue. Export and apply share it.
func findTemplate(g *Globals, id, token string) (*templates.TemplateDetail, bool) {
	id = strings.TrimSpace(id)

	if id == "" {
		items, ok := sendTemplatesRequest(g.api.baseURL, false, "", token, g)
		if !ok {
			return nil, false
		}

		if len(items) == 0 {
			fmt.Println("no template in the catalogue yet.")
			return nil, false
		}

		fmt.Println()

		for i, item := range items {
			fmt.Printf("%3d. %s (%s) — %s\n", i+1, item.Name, item.Kind, item.ID)
		}

		fmt.Println()

		choice, ok := promptChoice("template")
		if !ok || choice > len(items) {
			return nil, false
		}

		return sendTemplateRequest(g.api.baseURL, items[choice-1].ID, token, g)
	}

	templateID, err := uuid.Parse(id)
	if err != nil {
		g.logger.Warn("that is not a template id. ", slog.String("id", id))
		return nil, false
	}

	return sendTemplateRequest(g.api.baseURL, templateID, token, g)
}

// askTemplateValues asks the user for the value of every variable the
// template declares.
func askTemplateValues(variables []templates.TemplateVariableData) (map[string]string, bool) {
	values := map[string]string{}

	for _, variable := range variables {
		label := variable.Prompt
		if label == "" {
			label = variable.Key
		}

		if variable.DefaultValue != "" {
			label = fmt.Sprintf("%s [%s]", label, variable.DefaultValue)
		}

		value := promptLine(fmt.Sprintf("%s: ", label))

		if value == "" {
			value = variable.DefaultValue
		}

		if variable.Required && value == "" {
			fmt.Printf("the template asks for {{%s}}, and it has no answer yet.\n", variable.Key)
			return nil, false
		}

		values[variable.Key] = value
	}

	return values, true
}

// substitute replaces the placeholders in a content with the values. A
// placeholder that has no value is left as it is: the template may use the
// braces for something else.
func substitute(content string, values map[string]string) string {
	return placeholderPattern.ReplaceAllStringFunc(content, func(match string) string {
		key := match[2 : len(match)-2]

		if value, ok := values[key]; ok {
			return value
		}

		return match
	})
}

// undeclaredPlaceholders returns the placeholders the files hold that the
// template does not declare.
func undeclaredPlaceholders(files []templateFile, variables []templates.TemplateVariableData) []string {
	declared := map[string]bool{}
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

// writeTemplateFiles writes the files of a template into a folder.
//
// Every path is checked before any file is written: a tree that is half
// written is worse than a tree that is not written at all.
func writeTemplateFiles(target string, files []templateFile, force bool) error {
	if err := checkTemplateFiles(target, files, force); err != nil {
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

		// A file that was already there keeps its own mode, so the
		// executable flag is set on its own.
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}

	return nil
}

// checkTemplateFiles answers whether the files of a template may be written
// into a folder: no path may land outside it, and no file may be overwritten
// unless the user asked for it.
//
// The check runs before the questions are asked: a user is not asked what a
// template wants to know when the template is not going to be written.
func checkTemplateFiles(target string, files []templateFile, force bool) error {
	var existing []string
	seen := make(map[string]bool, len(files))

	for _, file := range files {
		// Two entries at one path would write one over the other, and
		// whichever came last would win silently. The server refuses it at
		// publish; an older template on disk is caught here.
		if seen[file.Path] {
			return fmt.Errorf("the template writes %q twice", file.Path)
		}
		seen[file.Path] = true

		path := filepath.Join(target, filepath.FromSlash(file.Path))

		// The path comes from the server, and the server got it from
		// another user: it has to land inside the folder, and nowhere
		// else.
		if !helper.PathIsInside(target, path) {
			return fmt.Errorf("the path %q would land outside the folder", file.Path)
		}

		if force {
			continue
		}

		found, err := helper.PathExists(path)
		if err != nil {
			return err
		}

		if found {
			existing = append(existing, file.Path)
		}
	}

	if len(existing) > 0 {
		sort.Strings(existing)

		return fmt.Errorf("these files are already there: %s (use --force to overwrite them)", strings.Join(existing, ", "))
	}

	return nil
}
