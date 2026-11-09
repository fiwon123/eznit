//go:build integration

package templates

import (
	"errors"
	"testing"

	"github.com/fiwon123/eznit/internal/platform/dbtest"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newTemplates(t *testing.T) (*sqlRepository, *sqlx.DB) {
	t.Helper()

	db := dbtest.Open(t)
	dbtest.Reset(t, db)

	return NewRepository(db, dbtest.Logger(t)), db
}

func createAccount(t *testing.T, db *sqlx.DB, email string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := db.Get(&id,
		"INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id",
		email, []byte("hash")); err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	return id
}

func createCommand(t *testing.T, db *sqlx.DB, userID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	id := uuid.New()
	if _, err := db.Exec(
		"INSERT INTO commands (id, user_id, name, content) VALUES ($1, $2, $3, $4)",
		id, userID, name, "#!/bin/sh\n"); err != nil {
		t.Fatalf("create command %s: %v", name, err)
	}
	return id
}

// fixture is one complete template: two text files, two variables and an
// optional command reference.
func fixture(t *testing.T, templateID, userID uuid.UUID, slug string, commandIDs ...uuid.UUID) (Template, []TemplateFile, []TemplateVariable, []TemplateCommand) {
	t.Helper()

	template := Template{
		ID:          templateID,
		UserID:      userID,
		Name:        "Starter",
		Slug:        slug,
		Description: "a starter template",
		Kind:        "opencode",
		Version:     1,
		IsPublic:    false,
	}

	files := []TemplateFile{
		{ID: uuid.New(), TemplateID: templateID, Path: ".opencode/agent.md", Content: "agent", Kind: "text"},
		{ID: uuid.New(), TemplateID: templateID, Path: "README.md", Content: "hello", Kind: "text"},
	}
	variables := []TemplateVariable{
		{ID: uuid.New(), TemplateID: templateID, Key: "PROJECT_NAME", Prompt: "name?", DefaultValue: "demo", Position: 0},
		{ID: uuid.New(), TemplateID: templateID, Key: "LICENSE", Prompt: "license?", DefaultValue: "MIT", Position: 1},
	}

	var commands []TemplateCommand
	for _, commandID := range commandIDs {
		commands = append(commands, TemplateCommand{
			TemplateID: templateID,
			CommandID:  commandID,
			TargetPath: "bin/fmt",
		})
	}

	return template, files, variables, commands
}

func TestCreateTemplateWithContents(t *testing.T) {
	repo, db := newTemplates(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	commandID := createCommand(t, db, alice, "fmt")
	templateID := uuid.New()
	template, files, variables, commands := fixture(t, templateID, alice, "starter", commandID)

	if err := repo.CreateTemplate(template, files, variables, commands); err != nil {
		t.Fatalf("CreateTemplate() error = %v", err)
	}

	got, err := repo.GetTemplate(templateID)
	if err != nil {
		t.Fatalf("GetTemplate() error = %v", err)
	}
	if got.Name != "Starter" || got.Slug != "starter" || got.Kind != "opencode" {
		t.Errorf("template = %+v, want the values that were written", got)
	}
	if got.OwnerEmail != "alice@example.com" {
		t.Errorf("owner_email = %q, want alice@example.com", got.OwnerEmail)
	}
	if got.FileCount != 2 {
		t.Errorf("file_count = %d, want 2", got.FileCount)
	}

	storedFiles, err := repo.GetTemplateFiles(templateID)
	if err != nil {
		t.Fatalf("GetTemplateFiles() error = %v", err)
	}
	if len(storedFiles) != 2 {
		t.Fatalf("files = %d, want 2", len(storedFiles))
	}
	// Ordered by path, so the tree is stable between reads.
	if storedFiles[0].Path != ".opencode/agent.md" || storedFiles[1].Path != "README.md" {
		t.Errorf("file order = %q, %q, want .opencode/agent.md then README.md", storedFiles[0].Path, storedFiles[1].Path)
	}

	storedVariables, err := repo.GetTemplateVariables(templateID)
	if err != nil {
		t.Fatalf("GetTemplateVariables() error = %v", err)
	}
	if len(storedVariables) != 2 || storedVariables[0].Key != "PROJECT_NAME" {
		t.Errorf("variables = %+v, want PROJECT_NAME first", storedVariables)
	}

	storedCommands, err := repo.GetTemplateCommands(templateID)
	if err != nil {
		t.Fatalf("GetTemplateCommands() error = %v", err)
	}
	if len(storedCommands) != 1 {
		t.Fatalf("commands = %d, want 1", len(storedCommands))
	}
	if storedCommands[0].Name != "fmt" || storedCommands[0].OwnerID != alice {
		t.Errorf("command = %+v, want fmt owned by alice", storedCommands[0])
	}

	// Visibility: the catalogue is public ∪ the caller's own (migration 000009).
	if list, err := repo.ListTemplates(alice, "", 50); err != nil || len(list) != 1 {
		t.Errorf("ListTemplates(alice) = %d, %v, want 1", len(list), err)
	}
	if list, err := repo.ListTemplates(bob, "", 50); err != nil || len(list) != 0 {
		t.Errorf("ListTemplates(bob) = %d, %v, want 0 for a private template", len(list), err)
	}
	if list, err := repo.ListTemplatesForUser(alice); err != nil || len(list) != 1 {
		t.Errorf("ListTemplatesForUser(alice) = %d, %v, want 1", len(list), err)
	}
	if list, err := repo.ListTemplatesForUser(bob); err != nil || len(list) != 0 {
		t.Errorf("ListTemplatesForUser(bob) = %d, %v, want 0", len(list), err)
	}

	// Publishing puts it in every catalogue, and the kind filter then applies.
	template.IsPublic = true
	if err := repo.UpdateTemplate(template, files, variables, commands); err != nil {
		t.Fatalf("UpdateTemplate(publish) error = %v", err)
	}
	if list, err := repo.ListTemplates(bob, "opencode", 50); err != nil || len(list) != 1 {
		t.Errorf("ListTemplates(bob, opencode) = %d, %v, want 1", len(list), err)
	}
	if list, err := repo.ListTemplates(bob, "codex", 50); err != nil || len(list) != 0 {
		t.Errorf("ListTemplates(bob, codex) = %d, %v, want 0", len(list), err)
	}

	if owned, err := repo.OwnedCommandIDs(alice, []uuid.UUID{commandID}); err != nil || !owned[commandID] {
		t.Errorf("OwnedCommandIDs(alice) = %v, %v, want the command", owned, err)
	}
	if owned, err := repo.OwnedCommandIDs(bob, []uuid.UUID{commandID}); err != nil || owned[commandID] {
		t.Errorf("OwnedCommandIDs(bob) = %v, %v, want nothing", owned, err)
	}

	if exists, err := repo.SlugExists("starter"); err != nil || !exists {
		t.Errorf("SlugExists(starter) = %v, %v, want true", exists, err)
	}
	if exists, err := repo.SlugExists("other"); err != nil || exists {
		t.Errorf("SlugExists(other) = %v, %v, want false", exists, err)
	}
	if _, err := repo.GetTemplate(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetTemplate(missing) error = %v, want ErrNotFound", err)
	}
}

// The transaction is what keeps a broken template out of the catalogue: a
// duplicate path fails on its own, and the template row must not survive it.
func TestCreateTemplateRollsBackOnBadFile(t *testing.T) {
	repo, db := newTemplates(t)

	alice := createAccount(t, db, "alice@example.com")

	templateID := uuid.New()
	template, files, variables, _ := fixture(t, templateID, alice, "broken")
	files[1].Path = files[0].Path // UNIQUE (template_id, path)

	if err := repo.CreateTemplate(template, files, variables, nil); err == nil {
		t.Fatal("CreateTemplate(duplicate path) error = nil, want the UNIQUE refusal")
	}

	if _, err := repo.GetTemplate(templateID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the template row survived the rollback: error = %v", err)
	}
	if n := dbtest.Count(t, db, "template_files", "template_id = $1", templateID); n != 0 {
		t.Errorf("template_files rows = %d after the rollback, want 0", n)
	}
}

// A slug is unique across the instance, not per user: it identifies a template
// in a URL.
func TestSlugIsUniqueAcrossAccounts(t *testing.T) {
	repo, db := newTemplates(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	first, files, variables, _ := fixture(t, uuid.New(), alice, "taken")
	if err := repo.CreateTemplate(first, files, variables, nil); err != nil {
		t.Fatalf("CreateTemplate(first) error = %v", err)
	}

	second, files, variables, _ := fixture(t, uuid.New(), bob, "taken")
	if err := repo.CreateTemplate(second, files, variables, nil); err == nil {
		t.Fatal("CreateTemplate(duplicate slug) error = nil, want the UNIQUE refusal")
	}
}

// The schema's CHECK is the second gate behind the service's kind list: a
// template row with a kind this build knows must be insertable. The shell
// kinds were added by migration 000015, so this test fails if that migration
// and the code drift apart.
func TestEveryTemplateKindIsAcceptedByTheSchema(t *testing.T) {
	repo, db := newTemplates(t)

	alice := createAccount(t, db, "alice@example.com")

	for _, kind := range Kinds {
		template := Template{
			ID:     uuid.New(),
			UserID: alice,
			Name:   "a " + kind + " template",
			Slug:   "a-" + kind + "-template",
			Kind:   kind,
		}

		if err := repo.CreateTemplate(template, nil, nil, nil); err != nil {
			t.Errorf("CreateTemplate(kind %q) error = %v, want the schema to accept it", kind, err)
		}
	}
}

func TestDeleteTemplateCascadesItsContents(t *testing.T) {
	repo, db := newTemplates(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	commandID := createCommand(t, db, alice, "fmt")
	templateID := uuid.New()
	template, files, variables, commands := fixture(t, templateID, alice, "owned", commandID)

	if err := repo.CreateTemplate(template, files, variables, commands); err != nil {
		t.Fatalf("CreateTemplate() error = %v", err)
	}

	if err := repo.DeleteTemplate(templateID, bob); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteTemplate(other account) error = %v, want ErrNotFound", err)
	}

	if err := repo.DeleteTemplate(templateID, alice); err != nil {
		t.Fatalf("DeleteTemplate(owner) error = %v", err)
	}

	for _, check := range []struct {
		table     string
		condition string
	}{
		{"templates", "id = $1"},
		{"template_files", "template_id = $1"},
		{"template_variables", "template_id = $1"},
		{"template_commands", "template_id = $1"},
	} {
		if n := dbtest.Count(t, db, check.table, check.condition, templateID); n != 0 {
			t.Errorf("%s rows = %d after the delete, want 0", check.table, n)
		}
	}

	// The command itself belongs to the library, not the template.
	if n := dbtest.Count(t, db, "commands", "id = $1", commandID); n != 1 {
		t.Errorf("commands rows = %d, want 1: deleting a template must not delete its command", n)
	}
}
