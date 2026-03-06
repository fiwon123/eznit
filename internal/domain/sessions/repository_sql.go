package sessions

import (
	"log/slog"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/jmoiron/sqlx"
	"github.com/oklog/ulid/v2"
)

type sqlRepository struct {
	db     *sqlx.DB
	logger *logger.Config
}

func NewRepository(db *sqlx.DB, logger *logger.Config) *sqlRepository {
	return &sqlRepository{
		db:     db,
		logger: logger,
	}
}
func (r *sqlRepository) GetSession(token string) *Session {
	var session Session

	err := r.db.Get(&session, "SELECT * FROM sessions WHERE token=$1", token)
	if err != nil {
		r.logger.Error("get session", slog.Any("error", err))
		return nil
	}

	return &session
}

func (r *sqlRepository) GetSessionByUserID(userID ulid.ULID) *Session {
	var session Session

	err := r.db.Select(&session, "SELECT * FROM sessions WHERE user_id=$1", userID)
	if err != nil {
		r.logger.Error("get session by user id", slog.Any("error", err))
		return nil
	}

	return &session
}

func (r *sqlRepository) CreateSession(s Session) bool {
	_, err := r.db.NamedExec("INSERT INTO sessions(token, user_id, expires_at) VALUES (:token, :user_id, :expires_at)", s)
	if err != nil {
		r.logger.Error("create session", slog.Any("error", err))
		return false
	}

	return true
}

func (r *sqlRepository) UpdateSession(s Session) bool {
	exec := "UPDATE sessions SET is_active=:is_active WHERE token=:token"

	_, err := r.db.NamedExec(exec, s)
	if err != nil {
		r.logger.Error("update session", slog.Any("error", err))
		return false
	}

	return true
}

func (r *sqlRepository) GetUserIDByToken(s string) (ulid.ULID, bool) {
	exec := `SELECT id FROM users u
			INNER JOIN sessions s ON u.id = s.user_id
			WHERE s.token = $1`

	var userID ulid.ULID

	row := r.db.QueryRow(exec, s)
	if err := row.Scan(&userID); err != nil {
		r.logger.Error("get user id by token", slog.Any("error", err))
		return ulid.ULID{}, false
	}

	return userID, true
}
