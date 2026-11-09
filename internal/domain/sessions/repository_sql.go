package sessions

import (
	"database/sql"
	"errors"
	"log/slog"

	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// sessionColumns is the column list every read of the sessions table names.
// It is named, not `*`, for the same reason as files: a migration that adds a
// column must not break a read at run time. sessions/schema_test.go checks it
// against the struct and the migrations.
const sessionColumns = "token, user_id, is_active, created_at, expires_at"

// Session repository implementation
type sqlRepository struct {
	db     *sqlx.DB
	logger *logger.Config
}

// Return a new session repository
func NewRepository(db *sqlx.DB, logger *logger.Config) *sqlRepository {
	return &sqlRepository{
		db:     db,
		logger: logger,
	}
}

// Get session by token
func (r *sqlRepository) GetSession(token string) (*Session, error) {
	var session Session

	err := r.db.Get(&session, "SELECT "+sessionColumns+" FROM sessions WHERE token=$1", token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		r.logger.Error("get session", slog.Any("error", err))
		return nil, err
	}

	return &session, nil
}

// Get session by user id
func (r *sqlRepository) GetSessionByUserID(userID uuid.UUID) (*Session, error) {
	r.logger.Logger.Debug("GetSessionByUserID")
	var session Session

	err := r.db.Get(&session, "SELECT "+sessionColumns+" FROM sessions WHERE user_id=$1 ORDER BY created_at DESC LIMIT 1", userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		r.logger.Error("get session by user id", slog.Any("error", err))
		return nil, err
	}

	return &session, nil
}

// Create a new session using session model
func (r *sqlRepository) CreateSession(s Session) error {
	_, err := r.db.NamedExec("INSERT INTO sessions(token, user_id, expires_at) VALUES (:token, :user_id, :expires_at)", s)
	if err != nil {
		r.logger.Error("create session", slog.Any("error", err))
		return err
	}

	return nil
}

// Update session using session model
func (r *sqlRepository) UpdateSession(s Session) error {
	result, err := r.db.NamedExec("UPDATE sessions SET is_active=:is_active WHERE token=:token", s)
	if err != nil {
		r.logger.Error("update session", slog.Any("error", err))
		return err
	}

	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return ErrNotFound
	}

	return nil
}

// Get user id by token
func (r *sqlRepository) GetUserIDByToken(token string) (uuid.UUID, error) {
	r.logger.Debug("GetUserIDByToken", slog.String("token_fp", helper.Fingerprint(token)))

	exec := `SELECT u.id FROM users u
			INNER JOIN sessions s ON u.id = s.user_id
			WHERE s.token = $1`

	var userID uuid.UUID

	if err := r.db.QueryRow(exec, token).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.UUID{}, ErrNotFound
		}
		r.logger.Error("get user id by token", slog.Any("error", err))
		return uuid.UUID{}, err
	}

	return userID, nil
}
