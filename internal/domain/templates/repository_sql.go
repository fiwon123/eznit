package templates

import (
	"log/slog"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Templates Repository implementation
type sqlRepository struct {
	db     *sqlx.DB
	logger *logger.Config
}

// Return a new templates repository
func NewRepository(db *sqlx.DB, logger *logger.Config) *sqlRepository {
	return &sqlRepository{
		db:     db,
		logger: logger,
	}
}

// templateColumns is the read shape of a template, with the owner's email
// joined in: the catalogue shows who published what.
const templateColumns = `t.id, t.user_id, t.name, t.slug, t.description, t.kind,
	t.version, t.is_public, t.created_at, t.updated_at, u.email AS owner_email`

// templateFileColumns is the read shape of a file of a template, and
// templateVariableColumns the read shape of a variable.
//
// The columns are named, and not `*`: sqlx refuses to scan a column it cannot
// place in the struct, so a query that reads `*` breaks the day a migration
// adds a column — and it breaks at runtime, on a table nobody looks at until
// a user does.
const templateFileColumns = `id, template_id, path, content, is_executable`

const templateVariableColumns = `id, template_id, var_key, prompt, default_value, required, sort_order`

// ListTemplates returns the catalogue for a user: every public template and
// the user's own, public or not. The filter is in the query, so a template
// that is not visible to the caller is never read in the first place.
func (r *sqlRepository) ListTemplates(userID uuid.UUID, kind string, limit int) ([]Template, bool) {
	r.logger.Debug("listing templates", slog.String("userID", userID.String()))

	var templates []Template

	query := `SELECT ` + templateColumns + `
		FROM templates t
		JOIN users u ON u.id = t.user_id
		WHERE (t.is_public = TRUE OR t.user_id = $1::uuid)
		  AND ($2::text = '' OR t.kind = $2::text)
		ORDER BY t.updated_at DESC
		LIMIT $3`

	err := r.db.Select(&templates, query, userID, kind, limit)
	if err != nil {
		r.logger.Error("list templates query", slog.Any("error", err))
		return []Template{}, false
	}

	return templates, true
}

// ListTemplatesForUser returns the user's own templates, private ones
// included.
func (r *sqlRepository) ListTemplatesForUser(userID uuid.UUID) ([]Template, bool) {
	r.logger.Debug("listing the user's own templates", slog.String("userID", userID.String()))

	var templates []Template

	query := `SELECT ` + templateColumns + `
		FROM templates t
		JOIN users u ON u.id = t.user_id
		WHERE t.user_id = $1
		ORDER BY t.updated_at DESC`

	err := r.db.Select(&templates, query, userID)
	if err != nil {
		r.logger.Error("list templates for user query", slog.Any("error", err))
		return []Template{}, false
	}

	return templates, true
}

// GetTemplate returns a template by id, with the owner's email.
func (r *sqlRepository) GetTemplate(id uuid.UUID) (*Template, bool) {
	r.logger.Debug("getting template", slog.String("id", id.String()))

	var template Template

	query := `SELECT ` + templateColumns + `
		FROM templates t
		JOIN users u ON u.id = t.user_id
		WHERE t.id = $1`

	err := r.db.Get(&template, query, id)
	if err != nil {
		r.logger.Error("get template query", slog.Any("error", err))
		return nil, false
	}

	return &template, true
}

// GetTemplateFiles returns the files of a template, ordered by path so the
// tree is stable between two reads.
func (r *sqlRepository) GetTemplateFiles(templateID uuid.UUID) ([]TemplateFile, bool) {
	r.logger.Debug("getting template files", slog.String("templateID", templateID.String()))

	var files []TemplateFile

	err := r.db.Select(&files, "SELECT "+templateFileColumns+" FROM template_files WHERE template_id=$1 ORDER BY path", templateID)
	if err != nil {
		r.logger.Error("get template files query", slog.Any("error", err))
		return []TemplateFile{}, false
	}

	return files, true
}

// GetTemplateVariables returns the variables of a template, in the order they
// are asked.
func (r *sqlRepository) GetTemplateVariables(templateID uuid.UUID) ([]TemplateVariable, bool) {
	r.logger.Debug("getting template variables", slog.String("templateID", templateID.String()))

	var variables []TemplateVariable

	err := r.db.Select(&variables, "SELECT "+templateVariableColumns+" FROM template_variables WHERE template_id=$1 ORDER BY sort_order, var_key", templateID)
	if err != nil {
		r.logger.Error("get template variables query", slog.Any("error", err))
		return []TemplateVariable{}, false
	}

	return variables, true
}

// CreateTemplate stores the template, its files and its variables in one
// transaction. A template whose files did not make it is a broken template,
// and a half-written one is worse: the transaction is what keeps the three
// in step.
func (r *sqlRepository) CreateTemplate(template Template, files []TemplateFile, variables []TemplateVariable) bool {
	r.logger.Debug("creating template", slog.String("id", template.ID.String()))

	tx, err := r.db.Beginx()
	if err != nil {
		r.logger.Error("begin transaction for template", slog.Any("error", err))
		return false
	}
	defer tx.Rollback()

	query := `INSERT INTO templates (id, user_id, name, slug, description, kind, version, is_public)
		VALUES (:id, :user_id, :name, :slug, :description, :kind, :version, :is_public)`

	if _, err := tx.NamedExec(query, template); err != nil {
		r.logger.Error("insert into templates", slog.Any("error", err))
		return false
	}

	if err := insertTemplateFiles(tx, files); err != nil {
		r.logger.Error("insert into template_files", slog.Any("error", err))
		return false
	}

	if err := insertTemplateVariables(tx, variables); err != nil {
		r.logger.Error("insert into template_variables", slog.Any("error", err))
		return false
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error("commit template transaction", slog.Any("error", err))
		return false
	}

	return true
}

// UpdateTemplate replaces a template with the given files and variables, in
// one transaction: the old files and variables go, the new ones come.
func (r *sqlRepository) UpdateTemplate(template Template, files []TemplateFile, variables []TemplateVariable) bool {
	r.logger.Debug("updating template", slog.String("id", template.ID.String()))

	tx, err := r.db.Beginx()
	if err != nil {
		r.logger.Error("begin transaction for template", slog.Any("error", err))
		return false
	}
	defer tx.Rollback()

	query := `UPDATE templates
		SET name=:name, slug=:slug, description=:description, kind=:kind,
		    version=:version, is_public=:is_public, updated_at=NOW()
		WHERE id=:id`

	if _, err := tx.NamedExec(query, template); err != nil {
		r.logger.Error("update templates", slog.Any("error", err))
		return false
	}

	if _, err := tx.Exec("DELETE FROM template_files WHERE template_id=$1", template.ID); err != nil {
		r.logger.Error("delete template files", slog.Any("error", err))
		return false
	}

	if _, err := tx.Exec("DELETE FROM template_variables WHERE template_id=$1", template.ID); err != nil {
		r.logger.Error("delete template variables", slog.Any("error", err))
		return false
	}

	if err := insertTemplateFiles(tx, files); err != nil {
		r.logger.Error("insert into template_files", slog.Any("error", err))
		return false
	}

	if err := insertTemplateVariables(tx, variables); err != nil {
		r.logger.Error("insert into template_variables", slog.Any("error", err))
		return false
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error("commit template transaction", slog.Any("error", err))
		return false
	}

	return true
}

// insertTemplateFiles writes the files of a template.
func insertTemplateFiles(tx *sqlx.Tx, files []TemplateFile) error {
	if len(files) == 0 {
		return nil
	}

	query := `INSERT INTO template_files (id, template_id, path, content, is_executable)
		VALUES (:id, :template_id, :path, :content, :is_executable)`

	_, err := tx.NamedExec(query, files)

	return err
}

// insertTemplateVariables writes the variables of a template.
func insertTemplateVariables(tx *sqlx.Tx, variables []TemplateVariable) error {
	if len(variables) == 0 {
		return nil
	}

	query := `INSERT INTO template_variables (id, template_id, var_key, prompt, default_value, required, sort_order)
		VALUES (:id, :template_id, :var_key, :prompt, :default_value, :required, :sort_order)`

	_, err := tx.NamedExec(query, variables)

	return err
}

// DeleteTemplate removes a template owned by the given user. The files and
// the variables go with it: the schema cascades.
func (r *sqlRepository) DeleteTemplate(id uuid.UUID, userID uuid.UUID) bool {
	r.logger.Debug("deleting template", slog.String("id", id.String()))

	result, err := r.db.Exec("DELETE FROM templates WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		r.logger.Error("delete template", slog.Any("error", err))
		return false
	}

	rows, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("delete template rows affected", slog.Any("error", err))
		return false
	}

	return rows > 0
}

// IsOwner reports whether the user owns the template.
func (r *sqlRepository) IsOwner(id uuid.UUID, userID uuid.UUID) bool {
	r.logger.Debug("checking template owner", slog.String("id", id.String()), slog.String("userID", userID.String()))

	var count int

	err := r.db.Get(&count, "SELECT count(*) FROM templates WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		r.logger.Error("failed to verify the template owner", slog.Any("error", err))
		return false
	}

	return count > 0
}

// SlugExists reports whether a slug is already taken. A slug identifies a
// template in a URL, so two templates cannot share one.
func (r *sqlRepository) SlugExists(slug string) bool {
	r.logger.Debug("checking whether the slug exists", slog.String("slug", slug))

	var count int

	err := r.db.Get(&count, "SELECT count(*) FROM templates WHERE slug=$1", slug)
	if err != nil {
		r.logger.Error("failed to verify the slug", slog.Any("error", err))
		return false
	}

	return count > 0
}
