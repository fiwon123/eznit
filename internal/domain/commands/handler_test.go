package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/fiwon123/eznit/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// fakeSessionsRepository is an in-memory sessions.Repository. The guard is
// exercised for real here: the route must reject a request without a token
// before any handler runs.
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

func (r *fakeSessionsRepository) CreateSession(session sessions.Session) bool {
	r.sessions[session.Token] = session
	return true
}

func (r *fakeSessionsRepository) UpdateSession(s sessions.Session) bool {
	r.sessions[s.Token] = s
	return true
}

func (r *fakeSessionsRepository) GetUserIDByToken(token string) (uuid.UUID, bool) {
	session, ok := r.sessions[token]
	if !ok {
		return uuid.Nil, false
	}
	return session.UserID, true
}

// newTestRouter returns a router with the commands routes registered, plus the
// session the guard will accept.
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

	guard := middleware.NewGuard(sessions.NewService(newFakeSessionsRepository(session), log), log)
	handler := NewHandler(NewService(repo, t.TempDir(), log), guard, log)

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return router
}

// A request without the session token is refused before any handler runs.
func TestCommandsNeedASession(t *testing.T) {
	router := newTestRouter(t, newFakeRepository(), uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/v1/commands/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

// A command is created, listed, read and deleted through the routes.
func TestCommandRoutesRoundTrip(t *testing.T) {
	owner := uuid.New()
	router := newTestRouter(t, newFakeRepository(), owner)

	body := `{"name":"fmt","content":"#!/bin/sh\ngofmt -w ."}`

	req := httptest.NewRequest(http.MethodPost, "/v1/commands/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var created types.Envelope[CommandData]
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode the response: %v", err)
	}

	if created.Data.Name != "fmt" || created.Data.Kind != CommandKindText {
		t.Errorf("expected the created command, got %+v", created.Data)
	}

	// The CLI asks for "/v1/commands", without the trailing slash.
	req = httptest.NewRequest(http.MethodGet, "/v1/commands", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var list types.Envelope[ListResponse]
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode the listing: %v", err)
	}

	if list.Data.Total != 1 {
		t.Fatalf("expected 1 command, got %d", list.Data.Total)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/commands/"+created.Data.ID.String(), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/commands/"+created.Data.ID.String(), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/commands/"+created.Data.ID.String(), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d after the delete, got %d", http.StatusNotFound, rec.Code)
	}
}

// An id that is not an id is a bad request, not a server failure.
func TestCommandRoutesRefuseABadID(t *testing.T) {
	router := newTestRouter(t, newFakeRepository(), uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/v1/commands/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

// Another user's command answers with not found through the route too.
func TestCommandRoutesHideAnotherUsersCommand(t *testing.T) {
	commandID := uuid.New()
	repo := newFakeRepository(Command{ID: commandID, UserID: uuid.New(), Name: "fmt", Kind: CommandKindText})

	router := newTestRouter(t, repo, uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/v1/commands/"+commandID.String(), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}
