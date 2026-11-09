package users

import (
	"database/sql"
	"errors"
	"fmt"
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
func (r *sqlRepository) GetUsers() ([]User, error) {
	var users []User

	r.logger.Debug("GetUsers")

	query := "SELECT " + userColumns + " FROM users"
	rows, err := r.db.Query(query)
	if err != nil {
		r.logger.Error("get users rows", slog.Any("error", err))
		return nil, fmt.Errorf("read users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var user User
		err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin)
		if err != nil {
			r.logger.Error("scan row", slog.Any("error", err))
			return nil, fmt.Errorf("read users: %w", err)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error("read users rows", slog.Any("error", err))
		return nil, fmt.Errorf("read users: %w", err)
	}

	return users, nil
}

// Report whether the table holds any account, which is what makes the account
// being created the instance's first — and its admin.
func (r *sqlRepository) HasUsers() (bool, error) {
	r.logger.Debug("HasUsers")

	var exists bool

	query := "SELECT EXISTS (SELECT 1 FROM users)"
	if err := r.db.QueryRow(query).Scan(&exists); err != nil {
		r.logger.Error("has users", slog.Any("error", err))
		return false, fmt.Errorf("read users: %w", err)
	}

	return exists, nil
}

// Get only a user
func (r *sqlRepository) GetUser(id uuid.UUID) (*User, error) {
	r.logger.Debug("GetUser", slog.String("id", id.String()))

	query := "SELECT " + userColumns + " FROM users WHERE id=$1"
	row := r.db.QueryRow(query, id)

	var user User
	err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		r.logger.Error("get user", slog.Any("error", err))
		return nil, fmt.Errorf("read user: %w", err)
	}

	return &user, nil
}

// Get user by email
func (r *sqlRepository) GetUserByEmail(email string) (*User, error) {
	r.logger.Debug("GetUserByEmail", slog.String("email", email))

	query := "SELECT " + userColumns + " FROM users WHERE email=$1"
	row := r.db.QueryRow(query, email)

	var user User
	err := row.Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt, &user.IsAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		r.logger.Error("get user by email", slog.Any("error", err))
		return nil, fmt.Errorf("read user by email: %w", err)
	}

	return &user, nil
}

// Verify if user is already registered based on email
func (r *sqlRepository) UserExists(email string) (bool, error) {
	r.logger.Debug("UserExists", slog.String("email", email))

	var count int

	query := "SELECT COUNT(*) FROM users WHERE email=$1"
	if err := r.db.QueryRow(query, email).Scan(&count); err != nil {
		r.logger.Error("user exists", slog.Any("error", err))
		return false, fmt.Errorf("read user existence: %w", err)
	}

	return count > 0, nil
}

// createUserQuery writes an account, the admin flag included: the service
// decides the flag (the first account is an admin) and this is the only write
// that sets it. schema_test.go holds the column list to the struct.
const createUserQuery = "INSERT INTO users (email,password,is_admin) VALUES ($1, $2, $3)"

// updateUserQuery deliberately does not name is_admin: an account update
// changes the email and the password, and no request may grant the flag.
const updateUserQuery = "UPDATE users SET email=$2, password=$3 WHERE id=$1"

// Create a new user based on user model data
func (r *sqlRepository) CreateUser(user User) error {
	r.logger.Debug("CreateUser", slog.String("email", user.Email))

	if _, err := r.db.Exec(createUserQuery, user.Email, user.Password, user.IsAdmin); err != nil {
		r.logger.Error("create user", slog.Any("error", err))
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

// Report whether a template that is not the account's own pulls in one of its
// commands. That is the one delete the schema refuses
// (`template_commands.command_id` is ON DELETE RESTRICT), and the service
// answers 409 for it instead of letting the transaction fail. The API only lets
// a template reference its owner's commands, so this is data written outside
// it — defence in depth, not a case the routes can produce.
func (r *sqlRepository) CommandsInUseByOthers(userID uuid.UUID) (bool, error) {
	r.logger.Debug("CommandsInUseByOthers", slog.String("id", userID.String()))

	var inUse bool

	query := `SELECT EXISTS (
		SELECT 1
		FROM template_commands tc
		JOIN commands c ON c.id = tc.command_id
		JOIN templates t ON t.id = tc.template_id
		WHERE c.user_id = $1 AND t.user_id <> $1
	)`

	if err := r.db.QueryRow(query, userID).Scan(&inUse); err != nil {
		r.logger.Error("commands in use by others", slog.Any("error", err))
		return false, fmt.Errorf("read command references: %w", err)
	}

	return inUse, nil
}

// Delete an account and every row that hangs off it.
//
// The order matters and it is why this is one transaction: the templates go
// first (so the account's own references to its commands go with them), then
// the commands, then the user row, whose cascades sweep sessions, files and
// their history. The templates and commands are deleted here rather than left
// to the user row's cascade because their ids are the folders the caller has to
// remove from disk.
func (r *sqlRepository) DeleteUser(user User) (OwnerStorage, error) {
	r.logger.Debug("DeleteUser", slog.String("id", user.ID.String()))

	var owner OwnerStorage

	tx, err := r.db.Beginx()
	if err != nil {
		r.logger.Error("begin delete user", slog.Any("error", err))
		return owner, fmt.Errorf("delete user: %w", err)
	}
	defer tx.Rollback()

	templateIDs, err := deleteReturningIDs(tx, "DELETE FROM templates WHERE user_id=$1 RETURNING id", user.ID, r.logger)
	if err != nil {
		return owner, err
	}

	commandIDs, err := deleteReturningIDs(tx, "DELETE FROM commands WHERE user_id=$1 RETURNING id", user.ID, r.logger)
	if err != nil {
		return owner, err
	}

	result, err := tx.Exec("DELETE FROM users WHERE id=$1", user.ID)
	if err != nil {
		r.logger.Error("delete user", slog.Any("error", err))
		return owner, fmt.Errorf("delete user: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("delete user", slog.Any("error", err))
		return owner, fmt.Errorf("delete user: %w", err)
	}

	if deleted == 0 {
		// The account was read a moment ago, so this is a delete that raced
		// another one: there is nothing left to remove.
		r.logger.Warn("delete user affected no row", slog.String("id", user.ID.String()))
		return owner, ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		r.logger.Error("commit delete user", slog.Any("error", err))
		return owner, fmt.Errorf("delete user: %w", err)
	}

	owner.TemplateIDs = templateIDs
	owner.CommandIDs = commandIDs

	return owner, nil
}

// deleteReturningIDs runs a DELETE that names its removed rows and collects
// their ids. A query that fails is returned, so the caller rolls the
// transaction back.
func deleteReturningIDs(tx *sqlx.Tx, query string, userID uuid.UUID, log *logger.Config) ([]uuid.UUID, error) {
	rows, err := tx.Queryx(query, userID)
	if err != nil {
		log.Error("delete rows", slog.Any("error", err))
		return nil, fmt.Errorf("delete owned rows: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			log.Error("scan deleted id", slog.Any("error", err))
			return nil, fmt.Errorf("delete owned rows: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		log.Error("read deleted ids", slog.Any("error", err))
		return nil, fmt.Errorf("delete owned rows: %w", err)
	}

	return ids, nil
}

// update a user based on user model data
func (r *sqlRepository) UpdateUser(user User) error {
	r.logger.Debug("UpdateUser", slog.String("id", user.ID.String()))

	result, err := r.db.Exec(updateUserQuery, user.ID, user.Email, user.Password)
	if err != nil {
		r.logger.Error("update user", slog.Any("error", err))
		return fmt.Errorf("update user: %w", err)
	}

	updated, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("update user", slog.Any("error", err))
		return fmt.Errorf("update user: %w", err)
	}

	if updated == 0 {
		// The account was read a moment ago, so this is an update that raced a
		// delete: there is nothing left to update.
		return ErrNotFound
	}

	return nil
}
