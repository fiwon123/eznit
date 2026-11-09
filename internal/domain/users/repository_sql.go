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

// userColumns is the read shape of an account: every column the User struct
// holds, named one by one.
//
// The columns are named, and not `*`: sqlx refuses to scan a column it cannot
// place in the struct, so a query that reads `*` breaks the day a migration
// adds a column — and it breaks at runtime, on a table nobody looks at until a
// user does.
const userColumns = `id, email, password, created_at, updated_at, is_admin`

// Get a list of users
func (r *sqlRepository) GetUsers() ([]User, bool) {
	var users []User

	r.logger.Debug("GetUsers")

	query := "SELECT " + userColumns + " FROM users"
	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Error("get users rows", slog.Any("error", err))
		return nil, false
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin)
		if err != nil {
			r.logger.Error("scan row", slog.Any("error", err))
			return nil, false
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error("read users rows", slog.Any("error", err))
		return nil, false
	}

	return users, true
}

// Report whether the table holds any account, which is what makes the account
// being created the instance's first — and its admin.
func (r *sqlRepository) HasUsers() (bool, bool) {
	r.logger.Debug("HasUsers")

	var exists bool

	query := "SELECT EXISTS (SELECT 1 FROM users)"
	if err := r.db.QueryRow(query).Scan(&exists); err != nil {
		r.logger.Error("has users", slog.Any("error", err))
		return false, false
	}

	return exists, true
}

// Get only a user
func (r *sqlRepository) GetUser(id uuid.UUID) (*User, bool) {
	r.logger.Debug("GetUser", slog.String("id", id.String()))

	query := "SELECT " + userColumns + " FROM users WHERE id=$1"
	row := r.db.QueryRow(query, id)

	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin); err != nil {
		r.logger.Error("get user", slog.Any("error", err))
		return nil, false
	}

	return &user, true
}

// Get user by email
func (r *sqlRepository) GetUserByEmail(email string) (*User, bool) {
	r.logger.Debug("GetUserByEmail", slog.String("email", email))

	query := "SELECT " + userColumns + " FROM users WHERE email=$1"
	row := r.db.QueryRow(query, email)

	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin); err != nil {
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

// createUserQuery writes an account, the admin flag included: the service
// decides the flag (the first account is an admin) and this is the only write
// that sets it. schema_test.go holds the column list to the struct.
const createUserQuery = "INSERT INTO users (email,password,is_admin) VALUES ($1, $2, $3)"

// updateUserQuery deliberately does not name is_admin: an account update
// changes the email and the password, and no request may grant the flag.
const updateUserQuery = "UPDATE users SET email=$2, password=$3 WHERE id=$1"

// Create a new user based on user model data
func (r *sqlRepository) CreateUser(user User) bool {
	r.logger.Debug("CreateUser", slog.String("email", user.Email))

	_, err := r.db.Exec(createUserQuery, user.Email, user.Password, user.IsAdmin)
	if err != nil {
		r.logger.Error("create user", slog.Any("error", err))
		return false
	}

	return true
}

// Delete a user based on user model data
func (r *sqlRepository) DeleteUser(user User) bool {
	r.logger.Debug("DeleteUser", slog.String("id", user.ID.String()))

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
	r.logger.Debug("UpdateUser", slog.String("id", user.ID.String()))

	_, err := r.db.Exec(updateUserQuery, user.ID, user.Email, user.Password)
	if err != nil {
		r.logger.Error("update user", slog.Any("error", err))
		return false
	}

	return true
}
