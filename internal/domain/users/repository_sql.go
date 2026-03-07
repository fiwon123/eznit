package users

import (
	"log/slog"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// A implementation of users repository interface
// All useful function related to users
type sqlRepository struct {
	db     *sqlx.DB
	logger *logger.Config
}

// Return a new sqlRepository related to users
func NewRepository(db *sqlx.DB, logger *logger.Config) *sqlRepository {
	return &sqlRepository{
		db:     db,
		logger: logger,
	}
}

// Get a list of users
func (r *sqlRepository) GetUsers() ([]User, bool) {
	var users []User

	r.logger.Debug("GetUsers")

	query := "SELECT id,email,password,created_at FROM users"
	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Error("get users rows", slog.Any("error", err))
		return nil, false
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt)
		if err != nil {
			r.logger.Error("scan row", slog.Any("error", err))
			return nil, false
		}

		users = append(users, user)
	}

	return users, true
}

// Get only a user
func (r *sqlRepository) GetUser(id uuid.UUID) (*User, bool) {
	r.logger.Debug("GetUser", slog.String("id", id.String()))

	query := "SELECT id,email,password,created_at,updated_at FROM users WHERE id=$1"
	row := r.db.QueryRow(query, id)

	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt); err != nil {
		r.logger.Error("get user", slog.Any("error", err))
		return nil, false
	}

	return &user, true
}

// Get user by email
func (r *sqlRepository) GetUserByEmail(email string) (*User, bool) {
	r.logger.Debug("GetUserByEmail", slog.String("email", email))

	query := "SELECT id,email,password,created_at,updated_at FROM users WHERE email=$1"
	row := r.db.QueryRow(query, email)

	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt); err != nil {
		r.logger.Error("get user by email", slog.Any("error", err))
		return nil, false
	}

	return &user, true
}

// Verify if user is already registered based on email
func (r *sqlRepository) UserExists(email string) bool {
	r.logger.Debug("UserExists", slog.String("email", email))

	var count int

	query := "SELECT COUNT(*) FROM users WHERE email=$1"
	err := r.db.QueryRow(query, email).Scan(&count)

	if err != nil {
		r.logger.Error("user exists", slog.Any("error", err))
		return false
	}

	if count == 0 {
		r.logger.Debug("user not found")
		return false
	}

	return true
}

// Create a new user based on user model data
func (r *sqlRepository) CreateUser(user User) bool {
	r.logger.Debug("CreateUser", slog.Any("user", user))

	exec := "INSERT INTO users (email,password) VALUES ($1, $2)"

	_, err := r.db.Exec(exec, user.Email, user.Password)
	if err != nil {
		r.logger.Error("create user", slog.Any("error", err))
		return false
	}

	return true
}

// Delete a user based on user model data
func (r *sqlRepository) DeleteUser(user User) bool {
	r.logger.Debug("DeleteUser", slog.Any("user", user))

	exec := "DELETE FROM users WHERE id=$1"

	_, err := r.db.Exec(exec, user.ID)
	if err != nil {
		r.logger.Error("delete user", slog.Any("error", err))
		return false
	}

	return true
}

// update a user based on user model data
func (r *sqlRepository) UpdateUser(user User) bool {
	r.logger.Debug("UpdateUser", slog.Any("user", user))

	exec := "UPDATE users SET email=$2, password=$3 WHERE id=$1"

	_, err := r.db.Exec(exec, user.ID, user.Email, user.Password)
	if err != nil {
		r.logger.Error("update user", slog.Any("error", err))
		return false
	}

	return true
}
