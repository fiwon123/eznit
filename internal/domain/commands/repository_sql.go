package commands

import (
	"database/sql"
	"errors"
	"log/slog"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// commandColumns is the read shape of a command, named rather than `*`: a
// migration that adds a column must not break a read at run time.
// commands/schema_test.go checks it against the struct and the migrations.
const commandColumns = `id, user_id, name, kind, content, size, storage_path,
	is_executable, created_at, updated_at`

// Commands Repository implementation
type sqlRepository struct {
	db     *sqlx.DB
	logger *logger.Config
}

// Return a new commands repository
func NewRepository(db *sqlx.DB, logger *logger.Config) *sqlRepository {
	return &sqlRepository{
		db:     db,
		logger: logger,
	}
}

// ListCommands returns the caller's library.
func (r *sqlRepository) ListCommands(userID uuid.UUID) ([]Command, bool) {
	var commands []Command

	err := r.db.Select(&commands,
		"SELECT "+commandColumns+" FROM commands WHERE user_id=$1 ORDER BY name", userID)
	if err != nil {
		r.logger.Error("list commands query", slog.Any("error", err))
		return []Command{}, false
	}

	return commands, true
}

// GetCommand returns one command of the caller's library.
func (r *sqlRepository) GetCommand(id uuid.UUID, userID uuid.UUID) (*Command, bool) {
	var command Command

	err := r.db.Get(&command,
		"SELECT "+commandColumns+" FROM commands WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Error("get command query", slog.Any("error", err))
		}
		return nil, false
	}

	return &command, true
}

// GetCommandByName returns one command by its name.
func (r *sqlRepository) GetCommandByName(userID uuid.UUID, name string) (*Command, bool) {
	var command Command

	err := r.db.Get(&command,
		"SELECT "+commandColumns+" FROM commands WHERE user_id=$1 AND name=$2", userID, name)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Error("get command by name query", slog.Any("error", err))
		}
		return nil, false
	}

	return &command, true
}

// CreateCommand stores a command.
func (r *sqlRepository) CreateCommand(command Command) bool {
	query := `INSERT INTO commands (id, user_id, name, kind, content, size, storage_path, is_executable)
		VALUES (:id, :user_id, :name, :kind, :content, :size, :storage_path, :is_executable)`

	if _, err := r.db.NamedExec(query, command); err != nil {
		r.logger.Error("insert into commands", slog.Any("error", err))
		return false
	}

	return true
}

// UpdateCommand replaces a command owned by the caller.
func (r *sqlRepository) UpdateCommand(command Command) bool {
	query := `UPDATE commands SET name=:name, kind=:kind, content=:content, size=:size,
		storage_path=:storage_path, is_executable=:is_executable, updated_at=now()
		WHERE id=:id AND user_id=:user_id`

	result, err := r.db.NamedExec(query, command)
	if err != nil {
		r.logger.Error("update command", slog.Any("error", err))
		return false
	}

	affected, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("update command rows", slog.Any("error", err))
		return false
	}

	return affected == 1
}

// DeleteCommand removes a command owned by the caller.
func (r *sqlRepository) DeleteCommand(id uuid.UUID, userID uuid.UUID) bool {
	result, err := r.db.Exec("DELETE FROM commands WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		r.logger.Error("delete command", slog.Any("error", err))
		return false
	}

	affected, err := result.RowsAffected()
	if err != nil {
		r.logger.Error("delete command rows", slog.Any("error", err))
		return false
	}

	return affected == 1
}

// CommandInUse reports whether a template pulls the command in.
func (r *sqlRepository) CommandInUse(id uuid.UUID) bool {
	var inUse bool

	err := r.db.Get(&inUse,
		"SELECT EXISTS (SELECT 1 FROM template_commands WHERE command_id=$1)", id)
	if err != nil {
		r.logger.Error("command in use query", slog.Any("error", err))
		// An answer of "not in use" on a failed query would delete a
		// command a template needs, so a failure counts as in use.
		return true
	}

	return inUse
}
