package users

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type sqlRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *sqlRepository {
	return &sqlRepository{
		db: db,
	}
}

func (r *sqlRepository) GetUsers() []User {
	var users []User

	query := "SELECT id,email,password,created_at FROM users"
	rows, err := r.db.Query(query)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt)
		if err != nil {
			fmt.Println(err)
			return nil
		}

		users = append(users, user)
	}

	return users
}

func (r *sqlRepository) GetUser(id string) *User {

	query := "SELECT id,email,password,created_at FROM users WHERE id=$1"
	row := r.db.QueryRow(query, id)

	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		fmt.Println(err)
		return nil
	}

	return &user
}

func (r *sqlRepository) UserExists(email string) bool {
	var count int

	query := "SELECT COUNT(*) FROM users WHERE email=$1"
	err := r.db.QueryRow(query, email).Scan(&count)

	if err != nil {
		fmt.Println(err)
		return false
	}

	if count == 0 {
		return false
	}

	return true
}

func (r *sqlRepository) CreateUser(user User) bool {
	exec := "INSERT INTO users (email,password) VALUES ($1, $2)"

	_, err := r.db.Exec(exec, user.Email, user.Password)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (r *sqlRepository) DeleteUser(user User) bool {
	exec := "DELETE FROM users WHERE id=$1"

	_, err := r.db.Exec(exec, user.ID)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (r *sqlRepository) UpdateUser(user User) bool {
	exec := "UPDATE users SET email=$2, password=$3 WHERE id=$1"

	_, err := r.db.Exec(exec, user.ID, user.Email, user.Password)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}
