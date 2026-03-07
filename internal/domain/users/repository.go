package users

import (
	"time"

	"github.com/google/uuid"
)

// User Model
type User struct {
	ID        string    `db:"id"`
	Email     string    `db:"email"`
	Password  []byte    `db:"password"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// User repostory interface
type Repository interface {
	GetUsers() ([]User, bool)
	GetUser(id uuid.UUID) (*User, bool)
	GetUserByEmail(email string) (*User, bool)
	UserExists(email string) bool
	CreateUser(user User) bool
	DeleteUser(user User) bool
	UpdateUser(user User) bool
}
