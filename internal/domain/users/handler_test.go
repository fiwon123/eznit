package users

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// fakeSessionsRepository is an in-memory sessions.Repository. The guard is
// exercised for real here: a request without a token must be refused before any
// handler runs.
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

// newTestRouter returns a router with the users routes registered and one live
// session, for the account whose id is given.
//
// The guard is wired to the real service, which reads the fake repository: the
// admin answer travels the same path it does in production — guard to service
// to the account's is_admin column.
func newTestRouter(t *testing.T, repo Repository, userID uuid.UUID) *chi.Mux {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	session := sessions.Session{
		Token:     "test-token",
		UserID:    userID,
		IsActive:  true,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	sessionsService := sessions.NewService(newFakeSessionsRepository(session), log)
	service := NewService(repo, t.TempDir(), sessionsService, log)
	guard := middleware.NewGuard(sessionsService, service, log)

	handler := NewHandler(service, sessionsService, guard, log, RateLimitPolicy{})

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return router
}

// do sends a request with the test token, or without one when the token is "".
func do(t *testing.T, router *chi.Mux, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}

	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	return recorder
}

// The signup route answers the same for an address that already has an account
// as for a new one, and creates nothing.
func TestTheSignupRouteDoesNotRevealWhetherAnEmailIsRegistered(t *testing.T) {
	taken := User{ID: uuid.New(), Email: "taken@example.com"}
	repo := newFakeRepository(taken)
	router := newTestRouter(t, repo, uuid.New())

	body := `{"email":"taken@example.com","password":"secret","confirm_password":"secret"}`
	recorder := do(t, router, http.MethodPost, "/v1/signup", "", body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /v1/signup = %d, want %d", recorder.Code, http.StatusOK)
	}

	if len(repo.created) != 0 {
		t.Errorf("expected no account to be created, got %d", len(repo.created))
	}
}

// The login route answers the same whichever half of the pair is wrong.
func TestTheLoginRouteDoesNotRevealWhetherAnEmailIsRegistered(t *testing.T) {
	hash, err := hashPassword("right-password")
	if err != nil {
		t.Fatalf("failed to hash the test password: %v", err)
	}

	owner := User{ID: uuid.New(), Email: "owner@example.com", Password: hash}
	router := newTestRouter(t, newFakeRepository(owner), uuid.New())

	bodies := make([]string, 0, 2)

	for _, body := range []string{
		`{"email":"nobody@example.com","password":"whatever"}`,
		`{"email":"owner@example.com","password":"wrong"}`,
	} {
		recorder := do(t, router, http.MethodPost, "/v1/login", "", body)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("POST /v1/login = %d, want %d", recorder.Code, http.StatusUnauthorized)
		}

		bodies = append(bodies, recorder.Body.String())
	}

	if bodies[0] != bodies[1] {
		t.Errorf("the two failures answer differently: %q and %q", bodies[0], bodies[1])
	}
}

// The pair that matches still gets a token through the route, not only in the
// service: closing the oracle must not close the door.
func TestTheLoginRouteAnswersWithAToken(t *testing.T) {
	hash, err := hashPassword("right-password")
	if err != nil {
		t.Fatalf("failed to hash the test password: %v", err)
	}

	owner := User{ID: uuid.New(), Email: "owner@example.com", Password: hash}
	router := newTestRouter(t, newFakeRepository(owner), uuid.New())

	body := `{"email":"owner@example.com","password":"right-password"}`
	recorder := do(t, router, http.MethodPost, "/v1/login", "", body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /v1/login = %d, want %d", recorder.Code, http.StatusOK)
	}

	if !strings.Contains(recorder.Body.String(), "token") {
		t.Errorf("the answer carries no token: %s", recorder.Body.String())
	}
}

// The account list is the admin's view: a session that is not an admin does not
// get to enumerate the instance's accounts.
func TestTheUserListRefusesASessionThatIsNotAnAdmin(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	router := newTestRouter(t, newFakeRepository(owner), owner.ID)

	recorder := do(t, router, http.MethodGet, "/v1/users", "test-token", "")

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("GET /v1/users = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	if strings.Contains(recorder.Body.String(), owner.Email) {
		t.Error("the refusal leaked an account's email")
	}
}

// An admin reads the accounts.
func TestTheUserListAnswersAnAdmin(t *testing.T) {
	admin := User{ID: uuid.New(), Email: "admin@example.com", IsAdmin: true}
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	router := newTestRouter(t, newFakeRepository(admin, owner), admin.ID)

	recorder := do(t, router, http.MethodGet, "/v1/users", "test-token", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/users = %d, want %d", recorder.Code, http.StatusOK)
	}

	for _, email := range []string{admin.Email, owner.Email} {
		if !strings.Contains(recorder.Body.String(), email) {
			t.Errorf("the list does not name %q", email)
		}
	}
}

// Without a token the list is a 401, not a 403: the guard checks the session
// before it asks who the caller is.
func TestTheUserListRefusesAVisitor(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	router := newTestRouter(t, newFakeRepository(owner), owner.ID)

	recorder := do(t, router, http.MethodGet, "/v1/users", "", "")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("GET /v1/users = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

// The caller's own record is readable.
func TestGetUserAnswersTheCaller(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	router := newTestRouter(t, newFakeRepository(owner), owner.ID)

	recorder := do(t, router, http.MethodGet, "/v1/users/"+owner.ID.String(), "test-token", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /v1/users/{own id} = %d, want %d", recorder.Code, http.StatusOK)
	}

	if !strings.Contains(recorder.Body.String(), owner.Email) {
		t.Errorf("the answer does not name %q", owner.Email)
	}
}

// Another account's id is a 404, the same answer a missing id gets.
func TestGetUserHidesAnotherAccount(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	stranger := User{ID: uuid.New(), Email: "stranger@example.com"}
	router := newTestRouter(t, newFakeRepository(owner, stranger), owner.ID)

	recorder := do(t, router, http.MethodGet, "/v1/users/"+stranger.ID.String(), "test-token", "")

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/users/{another id} = %d, want %d", recorder.Code, http.StatusNotFound)
	}

	if strings.Contains(recorder.Body.String(), stranger.Email) {
		t.Error("the refusal leaked an account's email")
	}
}

// A write to an account is an admin's: a session that is not an admin cannot
// update one, and the route must not have changed anything.
func TestUpdateUserRefusesASessionThatIsNotAnAdmin(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	stranger := User{ID: uuid.New(), Email: "stranger@example.com"}
	repo := newFakeRepository(owner, stranger)
	router := newTestRouter(t, repo, owner.ID)

	body, err := json.Marshal(UpdateRequest{Id: stranger.ID, Email: "changed@example.com", Password: "secret"})
	if err != nil {
		t.Fatalf("failed to build the request body: %v", err)
	}

	recorder := do(t, router, http.MethodPut, "/v1/users/"+stranger.ID.String(), "test-token", string(body))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("PUT /v1/users/{id} = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	stored, err := repo.GetUser(stranger.ID)
	if err != nil {
		t.Fatalf("the refused update removed the account: %v", err)
	}

	if stored.Email != stranger.Email {
		t.Errorf("the refused update changed the account to %q", stored.Email)
	}
}

// An admin deletes an account, and the row is gone afterwards.
func TestDeleteUserLetsAnAdminDeleteAnAccount(t *testing.T) {
	admin := User{ID: uuid.New(), Email: "admin@example.com", IsAdmin: true}
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	repo := newFakeRepository(admin, owner)
	router := newTestRouter(t, repo, admin.ID)

	recorder := do(t, router, http.MethodDelete, "/v1/users/"+owner.ID.String(), "test-token", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /v1/users/{id} = %d, want %d", recorder.Code, http.StatusOK)
	}

	if _, err := repo.GetUser(owner.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the account was not deleted: %v", err)
	}
}

// The admin guard does not make an admin able to delete the account it is
// signed in with.
func TestTheDeleteRouteRefusesTheSignedInAccount(t *testing.T) {
	admin := User{ID: uuid.New(), Email: "admin@example.com", IsAdmin: true}
	repo := newFakeRepository(admin)
	router := newTestRouter(t, repo, admin.ID)

	recorder := do(t, router, http.MethodDelete, "/v1/users/"+admin.ID.String(), "test-token", "")

	if recorder.Code != http.StatusConflict {
		t.Fatalf("DELETE /v1/users/{own id} = %d, want %d", recorder.Code, http.StatusConflict)
	}

	if _, err := repo.GetUser(admin.ID); err != nil {
		t.Errorf("the refused delete removed the account: %v", err)
	}
}

// A delete by a session that is not an admin is refused, and the account stays.
func TestDeleteUserRefusesASessionThatIsNotAnAdmin(t *testing.T) {
	owner := User{ID: uuid.New(), Email: "owner@example.com"}
	stranger := User{ID: uuid.New(), Email: "stranger@example.com"}
	repo := newFakeRepository(owner, stranger)
	router := newTestRouter(t, repo, owner.ID)

	recorder := do(t, router, http.MethodDelete, "/v1/users/"+stranger.ID.String(), "test-token", "")

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("DELETE /v1/users/{id} = %d, want %d", recorder.Code, http.StatusForbidden)
	}

	if _, err := repo.GetUser(stranger.ID); err != nil {
		t.Errorf("the refused delete removed the account: %v", err)
	}
}
