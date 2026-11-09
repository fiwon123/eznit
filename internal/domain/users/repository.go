package users

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is what a read answers when there is no row.
//
// It is deliberately not the only answer a read can give: a query that failed
// reports its own error, so the service can tell "there is no such account"
// from "the database is down" (#148). The `(value, bool)` this replaces said
// both with false, and every caller turned a failure into "not found".
var ErrNotFound = errors.New("not found")

// User Model
//
// IsAdmin is what the admin guard asks about: the first account created on an
// instance is an admin, and the column is the only source of that answer.
type User struct {
	ID        uuid.UUID `db:"id"`
	Email     string    `db:"email"`
	Password  []byte    `db:"password"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	IsAdmin   bool      `db:"is_admin"`
}

// OwnerStorage is the on-disk half of a deleted account: the rows are gone, but
// the folders they named are not, and only the caller can remove them.
type OwnerStorage struct {
	// TemplateIDs are the account's templates: their binary files live under
	// `<upload-root>/templates/<template-id>/`.
	TemplateIDs []uuid.UUID
	// CommandIDs are the account's commands, whose binaries live under
	// `<upload-root>/commands/<command-id>/`.
	CommandIDs []uuid.UUID
}

// User repostory interface
//
// A read answers (value, error): ErrNotFound when there is no row, anything
// else when the question could not be asked. A write answers error, ErrNotFound
// when the row it acted on is gone.
type Repository interface {
	// HasUsers reports whether the table holds any account at all. The service
	// asks it once, at signup: the account created while the table is empty is
	// the instance's admin.
	HasUsers() (bool, error)
	GetUsers() ([]User, error)
	GetUser(id uuid.UUID) (*User, error)
	GetUserByEmail(email string) (*User, error)
	UserExists(email string) (bool, error)
	CreateUser(user User) error
	// CommandsInUseByOthers reports whether a template outside this account still
	// pulls in one of its commands. The schema refuses that delete
	// (`template_commands.command_id` is `ON DELETE RESTRICT`), so the service
	// asks first and answers 409 instead of a failed transaction.
	CommandsInUseByOthers(userID uuid.UUID) (bool, error)
	// DeleteUser removes the account and every row the schema cascades from it,
	// in one transaction. It returns the storage folders the caller has to
	// remove from disk.
	DeleteUser(user User) (OwnerStorage, error)
	UpdateUser(user User) error
}
