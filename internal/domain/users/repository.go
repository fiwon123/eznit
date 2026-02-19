package users

import "time"

type User struct {
	ID        string
	Email     string
	Password  string
	CreatedAt time.Time
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
