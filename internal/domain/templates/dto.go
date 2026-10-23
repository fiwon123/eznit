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
//
// A text file carries Content. A binary file carries ContentBase64 and no
// Content, so a client can write it without a second request; Size is what it
// weighed when it was published.
type TemplateFileData struct {
	ID            uuid.UUID `json:"id"`
	Path          string    `json:"path"`
	Kind          string    `json:"kind"`
	Size          int64     `json:"size"`
	Content       string    `json:"content,omitempty"`
	ContentBase64 string    `json:"content_base64,omitempty"`
	IsExecutable  bool      `json:"is_executable"`
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

// TemplateCommandData is a command a template pulls in from its owner's
// library, at the path it is written to.
type TemplateCommandData struct {
	CommandID     uuid.UUID `json:"command_id"`
	TargetPath    string    `json:"target_path"`
	Name          string    `json:"name"`
	Kind          string    `json:"kind"`
	Size          int64     `json:"size"`
	Content       string    `json:"content,omitempty"`
	ContentBase64 string    `json:"content_base64,omitempty"`
	IsExecutable  bool      `json:"is_executable"`
}

// TemplateDetail is a template with everything inside it: the files, the
// variables it declares and the commands it pulls in.
type TemplateDetail struct {
	TemplateData
	Files     []TemplateFileData     `json:"files"`
	Variables []TemplateVariableData `json:"variables"`
	Commands  []TemplateCommandData  `json:"commands"`
}

// ListResponse is the catalogue.
type ListResponse struct {
	Data  []TemplateData `json:"data"`
	Total int            `json:"total"`
}

// PublishFile is a file in a publish request.
//
// A text file sends Content. A binary file sends ContentBase64 — the request
// is JSON, and arbitrary bytes do not survive it — and the server writes the
// decoded bytes to disk. Kind defaults to "text", so a client that predates
// binary files keeps working.
type PublishFile struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	Content       string `json:"content"`
	ContentBase64 string `json:"content_base64"`
	IsExecutable  bool   `json:"is_executable"`
}

// PublishCommand is a reference to a command of the publisher's library, and
// the path that command takes inside the project.
type PublishCommand struct {
	CommandID  uuid.UUID `json:"command_id"`
	TargetPath string    `json:"target_path"`
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
	Commands    []PublishCommand  `json:"commands"`
	Variables   []PublishVariable `json:"variables"`
}
