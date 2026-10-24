package files

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
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

// multipartUpload returns a request body carrying one file field, plus the
// content type the handler expects for it.
func multipartUpload(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("failed to create the file field: %v", err)
	}

	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("failed to write the file field: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close the multipart body: %v", err)
	}

	return body, writer.FormDataContentType()
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

// The download is public for a shared file and not found for a private one:
// without a token the caller is a visitor, and the shared half of the rule is
// all a visitor gets.
func TestDownloadWithoutToken(t *testing.T) {
	owner := uuid.New()
	path := writeFileOnDisk(t, "the bytes")

	sharedID := uuid.New()
	privateID := uuid.New()

	repo := newFakeRepository(
		File{ID: sharedID, UserID: owner, Name: "shared", Ext: "txt", Path: path, IsPublic: true},
		File{ID: privateID, UserID: owner, Name: "secret", Ext: "txt", Path: path},
	)
	router := newTestRouter(t, repo, t.TempDir(), owner)

	t.Run("shared file serves the bytes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files/"+sharedID.String()+"/content", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
		}

		if body := rec.Body.String(); body != "the bytes" {
			t.Errorf("expected the file's bytes, got %q", body)
		}
	})

	t.Run("private file is not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files/"+privateID.String()+"/content", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d: %s", http.StatusNotFound, rec.Code, rec.Body.String())
		}

		if body := rec.Body.String(); body == "the bytes" {
			t.Error("the file content was served without a token")
		}
	})

	t.Run("missing file is not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/files/"+uuid.NewString()+"/content", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
		}
	})
}

// An unknown token on the public download route buys nothing: the route does
// not read it, and a private file still answers 404.
func TestDownloadWithAnInvalidTokenAnswersAsAVisitor(t *testing.T) {
	owner := uuid.New()
	path := writeFileOnDisk(t, "the bytes")

	fileID := uuid.New()
	repo := newFakeRepository(File{ID: fileID, UserID: owner, Name: "secret", Ext: "txt", Path: path})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	if body := rec.Body.String(); body == "the bytes" {
		t.Error("the file content was served for an invalid token")
	}
}

// The shared list is public: it answers the files their owners shared, and
// none of the private ones.
func TestSharedListWithoutTokenReturnsOnlyPublicFiles(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(
		File{ID: uuid.New(), UserID: owner, Name: "shared", Ext: "txt", IsPublic: true},
		File{ID: uuid.New(), UserID: owner, Name: "private-one", Ext: "txt"},
	)
	router := newTestRouter(t, repo, t.TempDir(), owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/shared", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if !strings.Contains(body, "shared") {
		t.Errorf("expected the shared file in the response, got %q", body)
	}

	if strings.Contains(body, "private-one") {
		t.Errorf("a private file is in the shared list: %q", body)
	}

	// The fake repository answers with this address on every row.
	if strings.Contains(body, "user@example.com") {
		t.Error("the owner's email is in a visitor's shared list")
	}
}

// A caller with a session still gets the attribution: the address is what the
// catalogue shows next to a shared file, and it stays for an account.
func TestSharedListWithATokenCarriesTheOwnerEmail(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(File{ID: uuid.New(), UserID: owner, Name: "shared", Ext: "txt", IsPublic: true})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/files/shared", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	if body := rec.Body.String(); !strings.Contains(body, "user@example.com") {
		t.Errorf("expected the owner's email in the response, got %q", body)
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

// A failed update answers exactly one body. The handler writes the error and
// then used to fall through to the success path, so the client got the error
// status followed by a second JSON document — two payloads, one response
// (#112). The second Decode is what fails when that `return` goes missing.
func TestUpdateThatFailsAnswersOneBody(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	fileID := uuid.New()

	// The caller is not the owner, so the service refuses the replacement.
	repo := newFakeRepository(File{ID: fileID, UserID: stranger, Name: "theirs", Ext: "txt"})
	router := newTestRouter(t, repo, t.TempDir(), owner)

	body, contentType := multipartUpload(t, "file", "replacement.txt", "the new bytes")

	req := httptest.NewRequest(http.MethodPut, "/v1/files/"+fileID.String()+"/", body)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d: %s", http.StatusConflict, rec.Code, rec.Body.String())
	}

	decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))

	var first map[string]any
	if err := decoder.Decode(&first); err != nil {
		t.Fatalf("the body is not a JSON document: %v", err)
	}

	if _, ok := first["error"]; !ok {
		t.Errorf("the body is not the error envelope: %v", first)
	}

	var second json.RawMessage
	if err := decoder.Decode(&second); !errors.Is(err, io.EOF) {
		t.Errorf("a second document follows the error: %q", rec.Body.String())
	}
}
