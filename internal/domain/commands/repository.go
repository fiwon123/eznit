package commands

import (
	"time"

	"github.com/google/uuid"
)

// The kinds of content a command holds. The values are the ones the schema's
// CHECK accepts, and they match a template file's kinds: the storage model is
// the same one (#34).
const (
	CommandKindText   = "text"
	CommandKindBinary = "binary"
)

// IsKind reports whether kind is one the service accepts.
func IsKind(kind string) bool {
	return kind == CommandKindText || kind == CommandKindBinary
}

// Command Model
type Command struct {
	ID     uuid.UUID `db:"id"`
	UserID uuid.UUID `db:"user_id"`
	Name   string    `db:"name"`
	Kind   string    `db:"kind"`
	// Content is the bytes of a text command. A binary command holds its
	// bytes on disk and this is empty.
	Content string `db:"content"`
	// Size is what the command weighed when it was written, so a listing
	// does not have to read every binary command back from disk.
	Size int64 `db:"size"`
	// StoragePath is where a binary command lives, relative to the upload
	// root, with forward slashes: `commands/<command-id>/<name>`.
	StoragePath  string    `db:"storage_path"`
	IsExecutable bool      `db:"is_executable"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	// Bytes carries the decoded content of a binary command from the
	// validation to the write. It is not a column.
	Bytes []byte `db:"-"`
}

// Repository is what the command service needs from storage.
type Repository interface {
	// ListCommands returns the caller's library, ordered by name.
	ListCommands(userID uuid.UUID) ([]Command, bool)
	// GetCommand returns one command of the caller's library. Another user's
	// command is not found.
	GetCommand(id uuid.UUID, userID uuid.UUID) (*Command, bool)
	// GetCommandByName returns one command by name. The name is unique per
	// user, so the pair identifies a row.
	GetCommandByName(userID uuid.UUID, name string) (*Command, bool)
	CreateCommand(command Command) bool
	UpdateCommand(command Command) bool
	DeleteCommand(id uuid.UUID, userID uuid.UUID) bool
	// CommandInUse reports whether a template pulls the command in. A
	// command that does is not deleted: the template would lose a file.
	CommandInUse(id uuid.UUID) bool
}
