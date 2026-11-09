//go:build integration

package sessions

import (
	"errors"
	"testing"
	"time"

	"github.com/fiwon123/eznit/internal/platform/dbtest"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newSessions(t *testing.T) (*sqlRepository, *Service, *sqlx.DB) {
	t.Helper()

	db := dbtest.Open(t)
	dbtest.Reset(t, db)

	log := dbtest.Logger(t)
	repo := NewRepository(db, log)

	return repo, NewService(repo, log), db
}

func createAccount(t *testing.T, db *sqlx.DB, email string) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	if err := db.Get(&id,
		"INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id",
		email, []byte("hash")); err != nil {
		t.Fatalf("create account %s: %v", email, err)
	}
	return id
}

func TestSessionRoundTrip(t *testing.T) {
	repo, service, db := newSessions(t)

	userID := createAccount(t, db, "alice@example.com")
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	if err := repo.CreateSession(Session{Token: "alice-token", UserID: userID, ExpiresAt: expires}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	session, err := repo.GetSession("alice-token")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if session.UserID != userID {
		t.Errorf("user_id = %s, want %s", session.UserID, userID)
	}
	if !session.IsActive {
		t.Error("is_active = false on a fresh session, want the column default")
	}
	if !session.ExpiresAt.Equal(expires) {
		t.Errorf("expires_at = %s, want %s", session.ExpiresAt, expires)
	}

	if !service.IsValid("alice-token") {
		t.Error("IsValid() = false for a live session, want true")
	}

	byUser, err := repo.GetSessionByUserID(userID)
	if err != nil || byUser.Token != "alice-token" {
		t.Errorf("GetSessionByUserID() = %+v, %v, want the session", byUser, err)
	}

	if got, err := repo.GetUserIDByToken("alice-token"); err != nil || got != userID {
		t.Errorf("GetUserIDByToken() = %s, %v, want %s", got, err, userID)
	}

	// Logging out is an update, not a delete: the row stays so a replayed
	// token is recognised and refused.
	if err := repo.UpdateSession(Session{Token: "alice-token", IsActive: false}); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}
	if service.IsValid("alice-token") {
		t.Error("IsValid() = true after logout, want false")
	}

	if _, err := repo.GetSession("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSession(missing) error = %v, want ErrNotFound", err)
	}
	if err := repo.UpdateSession(Session{Token: "missing"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateSession(missing) error = %v, want ErrNotFound", err)
	}
}

// A row can exist and still be refused: expiry and is_active are the service's
// rules, and the guard only asks the service.
func TestExpiredSessionIsNotValid(t *testing.T) {
	repo, service, db := newSessions(t)

	userID := createAccount(t, db, "alice@example.com")

	if err := repo.CreateSession(Session{
		Token:     "expired-token",
		UserID:    userID,
		ExpiresAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := repo.GetSession("expired-token"); err != nil {
		t.Fatalf("GetSession() error = %v: the row should still be readable", err)
	}
	if service.IsValid("expired-token") {
		t.Error("IsValid() = true for an expired session, want false")
	}
}

func TestSessionsCascadeOnAccountDelete(t *testing.T) {
	repo, _, db := newSessions(t)

	userID := createAccount(t, db, "alice@example.com")
	if err := repo.CreateSession(Session{
		Token:     "alice-token",
		UserID:    userID,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	if _, err := db.Exec("DELETE FROM users WHERE id = $1", userID); err != nil {
		t.Fatalf("delete the account: %v", err)
	}

	// Before migration 000014 this delete failed: sessions had no cascade.
	if n := dbtest.Count(t, db, "sessions", "user_id = $1", userID); n != 0 {
		t.Errorf("sessions left after the account delete = %d, want 0", n)
	}
	if _, err := repo.GetSession("alice-token"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSession() after the cascade error = %v, want ErrNotFound", err)
	}
}
