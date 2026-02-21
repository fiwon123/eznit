package users

import "time"

type User struct {
	ID        string    `db:"id"`
	Email     string    `db:"email"`
	Password  []byte    `db:"password"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Repository interface {
	GetUsers() []User
	GetUser(id string) *User
	GetUserByEmail(email string) *User
	UserExists(email string) bool
	CreateUser(user User) bool
	DeleteUser(user User) bool
	UpdateUser(user User) bool
}
