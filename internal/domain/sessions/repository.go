package sessions

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is what a read answers when there is no row, and what a write
// answers when the row it acted on is gone. It is distinct from a real failure
// so a caller can tell "no such session" from "the database is down".
var ErrNotFound = errors.New("not found")

// Session Model
type Session struct {
	Token     string    `db:"token"`
	UserID    uuid.UUID `db:"user_id"`
	IsActive  bool      `db:"is_active"`
	CreatedAt time.Time `db:"created_at"`
	ExpiresAt time.Time `db:"expires_at"`
}

// Session Repository Interface
//
// A read answers (value, error): ErrNotFound when there is no row, anything else
// when the question could not be asked. A write answers error, ErrNotFound when
// the row it acted on is gone.
type Repository interface {
	GetSession(token string) (*Session, error)
	GetSessionByUserID(userID uuid.UUID) (*Session, error)
	CreateSession(s Session) error
	UpdateSession(s Session) error
	GetUserIDByToken(token string) (uuid.UUID, error)
}
