package cli

import (
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
type TemplateApplyCmd struct {
	ID     string `arg:"" optional:"" help:"the id of the template"`
	Target string `help:"the folder to write the template into" default:"."`
	Force  bool   `help:"overwrite the files that are already there"`
}

// Run applies the template.
func (cmd *TemplateApplyCmd) Run(g *Globals) error {
	token, err := getToken()
	if err != nil {
		g.logger.Warn("not logged in. ")
		return nil
	}

	detail, ok := cmd.template(g, token)
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
	if err := checkTemplateFiles(target, templateFilesOf(detail), cmd.Force); err != nil {
		g.logger.Warn("the template was not applied. ", slog.String("error", err.Error()))
		return nil
	}

	// The values are asked for here, on the machine that writes the files:
	// a value the user types never travels to the server.
	values, ok := askTemplateValues(detail.Variables)
	if !ok {
		return nil
	}

	files := make([]templateFile, 0, len(detail.Files))

	for _, file := range detail.Files {
		files = append(files, templateFile{
			Path:         file.Path,
			Content:      substitute(file.Content, values),
			IsExecutable: file.IsExecutable,
		})
	}

	// A placeholder the template does not declare is left alone: it may be
	// something else entirely. It is named, so the user can see it.
	for _, key := range undeclaredPlaceholders(detail) {
		fmt.Printf("note: {{%s}} is in the files but the template does not declare it\n", key)
	}

	if err := writeTemplateFiles(target, files, cmd.Force); err != nil {
		g.logger.Warn("the template was not applied. ", slog.String("error", err.Error()))
		return nil
	}

	fmt.Println()
	fmt.Printf("applied %s to %s (%d files)\n", detail.Name, target, len(files))
	fmt.Println()

	return nil
}

// templateFilesOf turns the files of a template into the files the client
// writes: the same paths, and the contents as they came.
func templateFilesOf(detail *templates.TemplateDetail) []templateFile {
	files := make([]templateFile, 0, len(detail.Files))

	for _, file := range detail.Files {
		files = append(files, templateFile{
			Path:         file.Path,
			Content:      file.Content,
			IsExecutable: file.IsExecutable,
		})
	}

	return files
}

// template finds the template to apply: the one the user named, or the one
// they pick from the catalogue.
func (cmd *TemplateApplyCmd) template(g *Globals, token string) (*templates.TemplateDetail, bool) {
	id := strings.TrimSpace(cmd.ID)

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
func undeclaredPlaceholders(detail *templates.TemplateDetail) []string {
	declared := map[string]bool{}
	for _, variable := range detail.Variables {
		declared[variable.Key] = true
	}

	seen := map[string]bool{}
	var keys []string

	for _, file := range detail.Files {
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

		if err := os.WriteFile(path, []byte(file.Content), mode); err != nil {
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

	for _, file := range files {
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
