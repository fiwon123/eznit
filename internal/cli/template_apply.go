package cli

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/templates"
	"github.com/fiwon123/eznit/internal/templateapply"
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
	Files      []templateapply.File
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
	if err := templateapply.Check(target, src.Files, cmd.Force); err != nil {
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
			src.Files[i].Content = templateapply.Substitute(src.Files[i].Content, values)
		}
	}

	// A placeholder the template does not declare is left alone: it may be
	// something else entirely. It is named, so the user can see it.
	for _, key := range src.Undeclared {
		fmt.Printf("note: {{%s}} is in the files but the template does not declare it\n", key)
	}

	if err := templateapply.Write(target, src.Files, cmd.Force); err != nil {
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

	files, err := templateapply.Files(detail)
	if err != nil {
		g.logger.Warn("the template was not applied. ", slog.String("error", err.Error()))
		return nil, false
	}

	return &source{
		Name:       detail.Name,
		Files:      files,
		Variables:  detail.Variables,
		Undeclared: templateapply.Undeclared(files, detail.Variables),
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
		Undeclared: templateapply.Undeclared(files, variables),
	}, true
}

// filesFromBundle turns the files of a bundle into the files the client
// writes. The bundle was validated when it was read; the base64 decode here is
// where a binary file becomes bytes on disk.
func filesFromBundle(bundle *templates.Bundle) ([]templateapply.File, error) {
	files := make([]templateapply.File, 0, len(bundle.Files))

	for _, file := range bundle.Files {
		entry, err := templateapply.FileOf(file.Path, file.Kind, file.Content, file.ContentBase64, file.IsExecutable)
		if err != nil {
			return nil, err
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
