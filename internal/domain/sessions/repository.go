package sessions

import (
	"time"

	"github.com/oklog/ulid/v2"
)

type Session struct {
	Token     string    `db:"token"`
	UserID    string    `db:"user_id"`
	IsActive  bool      `db:"is_active"`
	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

type Repository interface {
	GetSession(token string) *Session
	GetSessionByUserID(userID ulid.ULID) *Session
	CreateSession(s Session) bool
	UpdateSession(s Session) bool
	GetUserIDByToken(s string) (ulid.ULID, bool)
}
