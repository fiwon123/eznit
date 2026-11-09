package users

import (
	"time"

	"github.com/google/uuid"
)

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
type Repository interface {
	// HasUsers reports whether the table holds any account at all, and whether
	// the question could be answered. The service asks it once, at signup: the
	// account created while the table is empty is the instance's admin.
	HasUsers() (bool, bool)
	GetUsers() ([]User, bool)
	GetUser(id uuid.UUID) (*User, bool)
	GetUserByEmail(email string) (*User, bool)
	UserExists(email string) bool
	CreateUser(user User) bool
	// CommandsInUseByOthers reports whether a template outside this account still
	// pulls in one of its commands, and whether the question could be answered.
	// The schema refuses that delete (`template_commands.command_id` is
	// `ON DELETE RESTRICT`), so the service asks first and answers 409 instead
	// of a failed transaction.
	CommandsInUseByOthers(userID uuid.UUID) (bool, bool)
	// DeleteUser removes the account and every row the schema cascades from it,
	// in one transaction. It returns the storage folders the caller has to
	// remove from disk.
	DeleteUser(user User) (OwnerStorage, bool)
	UpdateUser(user User) bool
}
