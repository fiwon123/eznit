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
	DeleteUser(user User) bool
	UpdateUser(user User) bool
}
