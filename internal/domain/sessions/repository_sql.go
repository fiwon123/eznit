package sessions

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
func (r *sqlRepository) GetSession(token string) *Session {
	var session Session

	err := r.db.Select(&session, "SELECT * FROM session WHERE token=$1", token)
	if err != nil {
		return nil
	}

	return &session
}

func (r *sqlRepository) GetSessionByUserID(userID string) *Session {
	var session Session

	err := r.db.Select(&session, "SELECT * FROM session WHERE user_id=$1", userID)
	if err != nil {
		return nil
	}

	return &session
}

func (r *sqlRepository) CreateSession(s Session) bool {
	_, err := r.db.NamedExec("INSERT INTO sessions(token, user_id, expires_at) VALUES (:token, :user_id, :created_at)", s)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (r *sqlRepository) UpdateSession(s Session) bool {
	exec := "UPDATE sessions SET is_active=:is_active WHERE token=:token"

	_, err := r.db.NamedExec(exec, s)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (r *sqlRepository) GetUserIDByToken(s string) (string, error) {
	exec := `SELECT id FROM users u
			INNER JOIN sessions s ON u.id = s.user_id
			WHERE s.token = $1`

	var userID string

	row := r.db.QueryRow(exec, s)
	if err := row.Scan(&userID); err != nil {
		fmt.Println(err)
		return "", err
	}

	return userID, nil
}
