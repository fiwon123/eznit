package db

import (
	"fmt"

	"github.com/fiwon123/eznit/internal/domain/model"
)

func (db *DbData) GetUsers() []model.User {
	var users []model.User

	query := "SELECT id,email,password,created_at FROM users"
	rows, err := db.sqlDB.Query(query)
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

func (db *DbData) CreateUser(user model.User) bool {
	exec := "INSERT INTO users (email,password) VALUES ($1, $2)"

	_, err := db.sqlDB.Exec(exec, user.Email, user.Password)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}
