package templates

import (
	"time"

	"github.com/google/uuid"
)

// Template Model
type Template struct {
	ID          uuid.UUID `db:"id"`
	UserID      uuid.UUID `db:"user_id"`
	Name        string    `db:"name"`
	Slug        string    `db:"slug"`
	Description string    `db:"description"`
	Kind        string    `db:"kind"`
	Version     int       `db:"version"`
	IsPublic    bool      `db:"is_public"`
	OwnerEmail  string    `db:"owner_email"`
	// FileCount is not a column of the table: the query counts the files
	// of the template, so the catalogue and the detail page cannot
	// disagree about how many files a template holds.
	FileCount int       `db:"file_count"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// The kinds of content a file of a template holds.
const (
	// FileKindText is a file whose bytes live in the `content` column.
	FileKindText = "text"
	// FileKindBinary is a file whose bytes live on disk, at StoragePath.
	FileKindBinary = "binary"
)

// IsFileKind reports whether kind is one the service accepts.
func IsFileKind(kind string) bool {
	return kind == FileKindText || kind == FileKindBinary
}

// TemplateFile Model
type TemplateFile struct {
	ID         uuid.UUID `db:"id"`
	TemplateID uuid.UUID `db:"template_id"`
	Path       string    `db:"path"`
	Content    string    `db:"content"`
	Kind       string    `db:"kind"`
	// Size is what the file weighed when it was published. It is stored rather
	// than computed so a listing can show it without reading every binary
	// file back from disk.
	Size int64 `db:"size"`
	// StoragePath is where a binary file lives, relative to the upload root,
	// with forward slashes: `templates/<template-id>/<file-id>/<name>`. It is
	// empty for a text file.
	StoragePath  string `db:"storage_path"`
	IsExecutable bool   `db:"is_executable"`
	// Bytes carries the decoded content of a binary file from the validation
	// to the write. It is not a column: the database holds the path, not the
	// bytes.
	Bytes []byte `db:"-"`
}

// TemplateVariable Model
type TemplateVariable struct {
	ID           uuid.UUID `db:"id"`
	TemplateID   uuid.UUID `db:"template_id"`
	Key          string    `db:"var_key"`
	Prompt       string    `db:"prompt"`
	DefaultValue string    `db:"default_value"`
	Required     bool      `db:"required"`
	Position     int       `db:"sort_order"`
}

// Templates repository interface
type Repository interface {
	// ListTemplates returns the catalogue for a user: the public templates
	// plus the user's own. An empty kind means every kind.
	ListTemplates(userID uuid.UUID, kind string, limit int) ([]Template, bool)
	// ListTemplatesForUser returns only the user's own templates, the
	// private ones included.
	ListTemplatesForUser(userID uuid.UUID) ([]Template, bool)
	GetTemplate(id uuid.UUID) (*Template, bool)
	GetTemplateFiles(templateID uuid.UUID) ([]TemplateFile, bool)
	GetTemplateVariables(templateID uuid.UUID) ([]TemplateVariable, bool)
	// CreateTemplate stores a template with its files and variables in one
	// transaction: a template without its files is not a template.
	CreateTemplate(template Template, files []TemplateFile, variables []TemplateVariable) bool
	// UpdateTemplate replaces a template with the given files and variables,
	// in one transaction.
	UpdateTemplate(template Template, files []TemplateFile, variables []TemplateVariable) bool
	DeleteTemplate(id uuid.UUID, userID uuid.UUID) bool
	IsOwner(id uuid.UUID, userID uuid.UUID) bool
	SlugExists(slug string) bool
}
