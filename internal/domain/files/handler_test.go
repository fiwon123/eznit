package files

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// newTestRouter returns a router with the files routes registered, plus the
// session the guard will accept.
func newTestRouter(t *testing.T, repo Repository, uploadFolder string, userID uuid.UUID) *chi.Mux {
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
	handler := NewHandler(NewService(repo, uploadFolder, log), guard, log)

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return router
}

// writeFileOnDisk creates a file with the given content and returns its path.
func writeFileOnDisk(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "stored.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write the test file: %v", err)
	}

	return path
}

// The list answers the caller's own files, so it is not public: without a
// token both routes are refused.
func TestListWithoutTokenIsRejected(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(File{ID: uuid.New(), UserID: owner, Name: "secret", Ext: "txt"})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	for _, path := range []string{"/v1/files/", "/v1/files/me"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
			}

			if !strings.Contains(rec.Body.String(), "No token provided") {
				t.Errorf("expected the session guard to reject the request, got %q", rec.Body.String())
			}
		})
	}
}

// Both list routes answer the caller's own files and nothing else: a file
// another user uploaded is not in the response, whichever route is asked.
func TestListReturnsOnlyTheCallersFiles(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	repo := newFakeRepository(
		File{ID: uuid.New(), UserID: owner, Name: "mine", Ext: "txt"},
		File{ID: uuid.New(), UserID: stranger, Name: "theirs", Ext: "txt"},
	)
	router := newTestRouter(t, repo, t.TempDir(), owner)

	for _, path := range []string{"/v1/files/", "/v1/files/me"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
			}

			body := rec.Body.String()
			if !strings.Contains(body, "mine") {
				t.Errorf("expected the caller's file in the response, got %q", body)
			}

			if strings.Contains(body, "theirs") {
				t.Errorf("another user's file is in the response: %q", body)
			}
		})
	}
}

// The download route is behind the session guard: without a token nothing is
// served.
//
// The body is asserted as well as the status, because the status alone does
// not say *who* rejected the request: the service also refuses a request
// that never went through the guard. The guard's own message is what proves
// the route sits inside the AuthUser group.
func TestDownloadWithoutTokenIsRejected(t *testing.T) {
	owner := uuid.New()
	path := writeFileOnDisk(t, "the bytes")

	repo := newFakeRepository(File{ID: uuid.New(), UserID: owner, Name: "secret", Ext: "txt", Path: path})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	fileID := uuid.New()
	repo.files[fileID] = File{ID: fileID, UserID: owner, Name: "secret", Ext: "txt", Path: path}

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID.String()+"/content", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if body := rec.Body.String(); body == "the bytes" {
		t.Error("the file content was served without a token")
	}

	if !strings.Contains(rec.Body.String(), "No token provided") {
		t.Errorf("expected the session guard to reject the request, got %q", rec.Body.String())
	}
}

// An unknown token is rejected the same way.
func TestDownloadWithAnInvalidTokenIsRejected(t *testing.T) {
	owner := uuid.New()
	path := writeFileOnDisk(t, "the bytes")

	repo := newFakeRepository(File{ID: uuid.New(), UserID: owner, Name: "secret", Ext: "txt", Path: path})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+uuid.NewString()+"/content", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

// A valid token for the wrong user does not download the file, and the bytes
// do not leak into the response.
func TestDownloadOfAnotherUsersFileIsNotFound(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	path := writeFileOnDisk(t, "the bytes")

	fileID := uuid.New()
	repo := newFakeRepository(File{ID: fileID, UserID: owner, Name: "secret", Ext: "txt", Path: path})
	router := newTestRouter(t, repo, t.TempDir(), stranger)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	if body := rec.Body.String(); body == "the bytes" {
		t.Error("the file content was served to a user who does not own it")
	}
}

// The owner still downloads their own file.
func TestDownloadOfTheOwnersFileServesTheContent(t *testing.T) {
	owner := uuid.New()
	path := writeFileOnDisk(t, "the bytes")

	fileID := uuid.New()
	repo := newFakeRepository(File{ID: fileID, UserID: owner, Name: "secret", Ext: "txt", Path: path})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if body := rec.Body.String(); body != "the bytes" {
		t.Errorf("expected the file content, got %q", body)
	}

	if disposition := rec.Header().Get("Content-Disposition"); disposition != "attachment; filename=secret.txt" {
		t.Errorf("unexpected Content-Disposition %q", disposition)
	}
}
