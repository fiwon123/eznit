package sessions

import (
	"time"

	"github.com/google/uuid"
)

// Session Model
type Session struct {
	Token     string    `db:"token"`
	UserID    uuid.UUID `db:"user_id"`
	IsActive  bool      `db:"is_active"`
	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

// Session Repository Interface
type Repository interface {
	GetSession(token string) *Session
	GetSessionByUserID(userID uuid.UUID) (*Session, bool)
	CreateSession(s Session) bool
	UpdateSession(s Session) bool
	GetUserIDByToken(s string) (uuid.UUID, bool)
}
