package db

import (
	"fmt"

	"github.com/fiwon123/eznit/internal/domain/model"
)

func (config *Config) GetUsers() []model.User {
	var users []model.User

	query := "SELECT id,email,password,created_at FROM users"
	rows, err := config.conn.Query(query)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var user model.User
		err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt)
		if err != nil {
			fmt.Println(err)
			return nil
		}

		users = append(users, user)
	}

	return users
}

func (config *Config) GetUser(id string) *model.User {

	query := "SELECT id,email,password,created_at FROM users WHERE id=$1"
	row := config.conn.QueryRow(query, id)

	var user model.User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		fmt.Println(err)
		return nil
	}

	return &user
}

func (config *Config) UserExists(email string) bool {
	var count int

	query := "SELECT COUNT(*) FROM users WHERE email=$1"
	err := config.conn.QueryRow(query, email).Scan(&count)

	if err != nil {
		fmt.Println(err)
		return false
	}

	if count == 0 {
		return false
	}

	return true
}

func (config *Config) CreateUser(user model.User) bool {
	exec := "INSERT INTO users (email,password) VALUES ($1, $2)"

	_, err := config.conn.Exec(exec, user.Email, user.Password)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (config *Config) DeleteUser(user model.User) bool {
	exec := "DELETE FROM users WHERE id=$1"

	_, err := config.conn.Exec(exec, user.ID)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (config *Config) UpdateUser(user model.User) bool {
	exec := "UPDATE users SET email=$2, password=$3 WHERE id=$1"

	_, err := config.conn.Exec(exec, user.ID, user.Email, user.Password)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}
