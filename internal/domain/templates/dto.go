package templates

import (
	"time"

	"github.com/google/uuid"
)

// The kinds of template: the AI terminal a template targets. The list is
// enforced by a CHECK constraint in the schema as well, so a template row
// cannot hold a kind the code does not know about.
const (
	KindOpencode = "opencode"
	KindCodex    = "codex"
	KindCopilot  = "copilot"
	KindCursor   = "cursor"
	KindWindsurf = "windsurf"
	KindGeneric  = "generic"
)

// Kinds is the set of kinds a template may carry.
var Kinds = []string{
	KindOpencode,
	KindCodex,
	KindCopilot,
	KindCursor,
	KindWindsurf,
	KindGeneric,
}

// IsKind reports whether kind is one the service accepts.
func IsKind(kind string) bool {
	for _, k := range Kinds {
		if k == kind {
			return true
		}
	}

	return false
}

// TemplateFileData is one file of a template, with the path it takes inside
// the project it is applied to.
type TemplateFileData struct {
	ID           uuid.UUID `json:"id"`
	Path         string    `json:"path"`
	Content      string    `json:"content"`
	IsExecutable bool      `json:"is_executable"`
}

// TemplateVariableData is one question a template asks when it is applied.
type TemplateVariableData struct {
	ID           uuid.UUID `json:"id"`
	Key          string    `json:"key"`
	Prompt       string    `json:"prompt"`
	DefaultValue string    `json:"default_value"`
	Required     bool      `json:"required"`
	Position     int       `json:"position"`
}

// TemplateData is the summary of a template: what the catalogue shows.
type TemplateData struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Kind        string    `json:"kind"`
	Version     int       `json:"version"`
	IsPublic    bool      `json:"is_public"`
	Owner       string    `json:"owner"`
	OwnerEmail  string    `json:"owner_email"`
	FileCount   int       `json:"file_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TemplateDetail is a template with everything inside it: the files and the
// variables it declares.
type TemplateDetail struct {
	TemplateData
	Files     []TemplateFileData     `json:"files"`
	Variables []TemplateVariableData `json:"variables"`
}

// ListResponse is the catalogue.
type ListResponse struct {
	Data  []TemplateData `json:"data"`
	Total int            `json:"total"`
}

// PublishFile is a file in a publish request.
type PublishFile struct {
	Path         string `json:"path"`
	Content      string `json:"content"`
	IsExecutable bool   `json:"is_executable"`
}

// PublishVariable is a variable in a publish request.
type PublishVariable struct {
	Key          string `json:"key"`
	Prompt       string `json:"prompt"`
	DefaultValue string `json:"default_value"`
	Required     bool   `json:"required"`
}

// PublishRequest is the body of a publish (POST) or an update (PUT).
//
// The same shape serves both: an update replaces the whole template, so the
// client never has to compute a diff and the server never has to merge one.
type PublishRequest struct {
	Name        string            `json:"name"`
	Slug        string            `json:"slug"`
	Description string            `json:"description"`
	Kind        string            `json:"kind"`
	IsPublic    bool              `json:"is_public"`
	Files       []PublishFile     `json:"files"`
	Variables   []PublishVariable `json:"variables"`
}
