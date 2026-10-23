package templates

import (
	"github.com/google/uuid"
)

// fakeRepository is an in-memory Repository. The rules under test — who may
// see a template, what a valid template is, and what the catalogue shows —
// live in the service, so the tests drive the service with a repository that
// needs no database.
type fakeRepository struct {
	templates map[uuid.UUID]Template
	files     map[uuid.UUID][]TemplateFile
	variables map[uuid.UUID][]TemplateVariable
	commands  map[uuid.UUID][]TemplateCommand
	// library is the command library the templates may reference, one
	// entry per row of the commands table.
	library    map[uuid.UUID]TemplateCommand
	createdIDs []uuid.UUID
	// createFails and updateFails make the write fail, for the tests that
	// check what the service does with the bytes when the rows are not
	// written.
	createFails bool
	updateFails bool
}

// newFakeRepository returns a repository holding the given templates.
func newFakeRepository(templates ...Template) *fakeRepository {
	repo := &fakeRepository{
		templates: map[uuid.UUID]Template{},
		files:     map[uuid.UUID][]TemplateFile{},
		variables: map[uuid.UUID][]TemplateVariable{},
		commands:  map[uuid.UUID][]TemplateCommand{},
		library:   map[uuid.UUID]TemplateCommand{},
	}

	for _, template := range templates {
		repo.templates[template.ID] = template
	}

	return repo
}

func (r *fakeRepository) ListTemplates(userID uuid.UUID, kind string, limit int) ([]Template, bool) {
	var visible []Template

	for _, template := range r.templates {
		if !template.IsPublic && template.UserID != userID {
			continue
		}
		if kind != "" && template.Kind != kind {
			continue
		}
		visible = append(visible, template)
	}

	return visible, true
}

func (r *fakeRepository) ListTemplatesForUser(userID uuid.UUID) ([]Template, bool) {
	var owned []Template

	for _, template := range r.templates {
		if template.UserID == userID {
			owned = append(owned, template)
		}
	}

	return owned, true
}

func (r *fakeRepository) GetTemplate(id uuid.UUID) (*Template, bool) {
	template, ok := r.templates[id]
	if !ok {
		return nil, false
	}

	return &template, true
}

func (r *fakeRepository) GetTemplateFiles(templateID uuid.UUID) ([]TemplateFile, bool) {
	return r.files[templateID], true
}

func (r *fakeRepository) GetTemplateVariables(templateID uuid.UUID) ([]TemplateVariable, bool) {
	return r.variables[templateID], true
}

// addCommand puts a command in the library, as if the user had created it.
func (r *fakeRepository) addCommand(command TemplateCommand) {
	r.library[command.CommandID] = command
}

func (r *fakeRepository) GetTemplateCommands(templateID uuid.UUID) ([]TemplateCommand, bool) {
	return r.commands[templateID], true
}

func (r *fakeRepository) OwnedCommandIDs(userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]bool, bool) {
	owned := make(map[uuid.UUID]bool, len(ids))

	for _, id := range ids {
		command, ok := r.library[id]
		if ok && command.OwnerID == userID {
			owned[id] = true
		}
	}

	return owned, true
}

func (r *fakeRepository) CreateTemplate(template Template, files []TemplateFile, variables []TemplateVariable, commands []TemplateCommand) bool {
	if r.createFails {
		return false
	}

	r.templates[template.ID] = template
	r.files[template.ID] = files
	r.variables[template.ID] = variables
	r.commands[template.ID] = r.resolveCommands(commands)
	r.createdIDs = append(r.createdIDs, template.ID)

	return true
}

// resolveCommands fills a reference in from the library, as the join in the
// SQL repository does.
func (r *fakeRepository) resolveCommands(commands []TemplateCommand) []TemplateCommand {
	resolved := make([]TemplateCommand, 0, len(commands))

	for _, command := range commands {
		if stored, ok := r.library[command.CommandID]; ok {
			command.Name = stored.Name
			command.Kind = stored.Kind
			command.Content = stored.Content
			command.Size = stored.Size
			command.StoragePath = stored.StoragePath
			command.IsExecutable = stored.IsExecutable
			command.OwnerID = stored.OwnerID
		}
		resolved = append(resolved, command)
	}

	return resolved
}

func (r *fakeRepository) UpdateTemplate(template Template, files []TemplateFile, variables []TemplateVariable, commands []TemplateCommand) bool {
	if r.updateFails {
		return false
	}

	r.templates[template.ID] = template
	r.files[template.ID] = files
	r.variables[template.ID] = variables
	r.commands[template.ID] = r.resolveCommands(commands)

	return true
}

func (r *fakeRepository) DeleteTemplate(id uuid.UUID, userID uuid.UUID) bool {
	template, ok := r.templates[id]
	if !ok || template.UserID != userID {
		return false
	}

	delete(r.templates, id)
	delete(r.files, id)
	delete(r.variables, id)
	delete(r.commands, id)

	return true
}

func (r *fakeRepository) IsOwner(id uuid.UUID, userID uuid.UUID) bool {
	template, ok := r.templates[id]
	return ok && template.UserID == userID
}

func (r *fakeRepository) SlugExists(slug string) bool {
	for _, template := range r.templates {
		if template.Slug == slug {
			return true
		}
	}

	return false
}
