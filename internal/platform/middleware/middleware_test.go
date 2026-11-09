package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// fakeSessionsRepository is an in-memory sessions.Repository, so the guard can
// be exercised without a database.
type fakeSessionsRepository struct {
	sessions map[string]sessions.Session
}

func newFakeSessionsRepository(s ...sessions.Session) *fakeSessionsRepository {
	repo := &fakeSessionsRepository{sessions: map[string]sessions.Session{}}
	for _, session := range s {
		repo.sessions[session.Token] = session
	}
	return repo
}

func (r *fakeSessionsRepository) GetSession(token string) *sessions.Session {
	session, ok := r.sessions[token]
	if !ok {
		return nil
	}
	return &session
}

func (r *fakeSessionsRepository) GetSessionByUserID(userID uuid.UUID) (*sessions.Session, bool) {
	for _, session := range r.sessions {
		if session.UserID == userID {
			return &session, true
		}
	}
	return nil, false
}

func (r *fakeSessionsRepository) CreateSession(s sessions.Session) bool {
	r.sessions[s.Token] = s
	return true
}

func (r *fakeSessionsRepository) UpdateSession(s sessions.Session) bool {
	r.sessions[s.Token] = s
	return true
}

func (r *fakeSessionsRepository) GetUserIDByToken(token string) (uuid.UUID, bool) {
	session, ok := r.sessions[token]
	if !ok {
		return uuid.UUID{}, false
	}
	return session.UserID, true
}

// newTestGuard returns a guard that accepts the given session, with no admin
// checker: its admin routes fail closed.
func newTestGuard(t *testing.T, session sessions.Session) *Guard {
	t.Helper()

	return newTestGuardWithAdmins(t, session, nil)
}

// newTestGuardWithAdmins is newTestGuard with an answer to the admin guard's
// question. A nil checker is what a domain that does not serve admin routes
// passes, and it is exercised on its own below.
func newTestGuardWithAdmins(t *testing.T, session sessions.Session, admins AdminChecker) *Guard {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewGuard(sessions.NewService(newFakeSessionsRepository(session), log), admins, log)
}

// fakeAdminChecker answers from a set of admin ids, so the guard can be
// exercised without a users service or a database.
type fakeAdminChecker struct {
	admins map[uuid.UUID]bool
}

func (c fakeAdminChecker) IsAdmin(_ context.Context, userID uuid.UUID) bool {
	return c.admins[userID]
}

// recorderHandler records whether the handler ran and which user the guard put
// in the context. The user is captured here because the request the guard
// hands over is not the one the caller built.
type recorderHandler struct {
	ran    bool
	userID uuid.UUID
}

func (h *recorderHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.ran = true
	h.userID, _ = r.Context().Value("user_id").(uuid.UUID)
	w.WriteHeader(http.StatusOK)
}

func TestOptionalUserLetsAVisitorThrough(t *testing.T) {
	owner := uuid.New()
	guard := newTestGuard(t, sessions.Session{
		Token:     "test-token",
		UserID:    owner,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/templates", nil)
	rec := httptest.NewRecorder()
	guard.OptionalUser(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !handler.ran {
		t.Fatal("the handler did not run for an anonymous request")
	}

	if handler.userID != uuid.Nil {
		t.Errorf("expected no user in the context, got %s", handler.userID)
	}
}

func TestOptionalUserResolvesAValidSession(t *testing.T) {
	owner := uuid.New()
	guard := newTestGuard(t, sessions.Session{
		Token:     "test-token",
		UserID:    owner,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/templates", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	guard.OptionalUser(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if handler.userID != owner {
		t.Errorf("expected the session's user %s, got %s", owner, handler.userID)
	}
}

// An unknown token is no token on a public route: the visitor view, not a 401.
func TestOptionalUserTreatsAnInvalidTokenAsAVisitor(t *testing.T) {
	owner := uuid.New()
	guard := newTestGuard(t, sessions.Session{
		Token:     "test-token",
		UserID:    owner,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/templates", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	guard.OptionalUser(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !handler.ran {
		t.Fatal("the handler did not run for an unknown token")
	}

	if handler.userID != uuid.Nil {
		t.Errorf("expected no user in the context, got %s", handler.userID)
	}
}

// A write keeps the strict guard: no token, no request.
func TestAuthUserStillRefusesAVisitor(t *testing.T) {
	owner := uuid.New()
	guard := newTestGuard(t, sessions.Session{
		Token:     "test-token",
		UserID:    owner,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodPost, "/v1/templates", nil)
	rec := httptest.NewRecorder()
	guard.AuthUser(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if handler.ran {
		t.Error("the handler ran without a token")
	}
}

// A live session whose account is an admin is admitted, and the handler gets
// the caller in its context — the admin routes read who acted.
func TestAuthAdminLetsAnAdminThrough(t *testing.T) {
	admin := uuid.New()
	guard := newTestGuardWithAdmins(t, sessions.Session{
		Token:     "test-token",
		UserID:    admin,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, fakeAdminChecker{admins: map[uuid.UUID]bool{admin: true}})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	guard.AuthAdmin(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if !handler.ran {
		t.Fatal("the handler did not run for an admin")
	}

	if handler.userID != admin {
		t.Errorf("expected the session's user %s, got %s", admin, handler.userID)
	}
}

// A real session is not enough: a non-admin is refused, and the handler does
// not run. 403 rather than 401, because the caller is authenticated.
func TestAuthAdminRefusesANonAdmin(t *testing.T) {
	owner := uuid.New()
	guard := newTestGuardWithAdmins(t, sessions.Session{
		Token:     "test-token",
		UserID:    owner,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, fakeAdminChecker{admins: map[uuid.UUID]bool{}})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	guard.AuthAdmin(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}

	if handler.ran {
		t.Error("the handler ran for a caller that is not an admin")
	}
}

// The admin guard is still a session guard: no token, no request.
func TestAuthAdminRefusesAVisitor(t *testing.T) {
	admin := uuid.New()
	guard := newTestGuardWithAdmins(t, sessions.Session{
		Token:     "test-token",
		UserID:    admin,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, fakeAdminChecker{admins: map[uuid.UUID]bool{admin: true}})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	rec := httptest.NewRecorder()
	guard.AuthAdmin(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if handler.ran {
		t.Error("the handler ran without a token")
	}
}

// An unknown token is a dead session, not a 403: the guard must not ask the
// admin question about a caller it never resolved.
func TestAuthAdminRefusesAnUnknownToken(t *testing.T) {
	admin := uuid.New()
	guard := newTestGuardWithAdmins(t, sessions.Session{
		Token:     "test-token",
		UserID:    admin,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	}, fakeAdminChecker{admins: map[uuid.UUID]bool{admin: true}})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	guard.AuthAdmin(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if handler.ran {
		t.Error("the handler ran for an unknown token")
	}
}

// A guard wired without a checker grants nothing: the routes exist, the answer
// is 403. Failing closed is the only safe default for a guard that is the
// difference between a session and an administrator.
func TestAuthAdminWithoutACheckerRefusesEverything(t *testing.T) {
	admin := uuid.New()
	guard := newTestGuard(t, sessions.Session{
		Token:     "test-token",
		UserID:    admin,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	handler := &recorderHandler{}
	req := httptest.NewRequest(http.MethodGet, "/v1/users", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	guard.AuthAdmin(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}

	if handler.ran {
		t.Error("the handler ran without an admin checker")
	}
}
