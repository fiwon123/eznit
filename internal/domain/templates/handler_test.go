package templates

import (
	"bytes"
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

// newTestRouter returns a router with the templates routes registered, plus
// the session the guard will accept.
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
	handler := NewHandler(NewService(repo, log), guard, log)

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return router
}

// The catalogue lists the templates a user may use, so it is not public:
// without a token it is refused.
func TestListWithoutTokenIsRejected(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(Template{ID: uuid.New(), UserID: owner, Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true})
	router := newTestRouter(t, repo, owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "No token provided") {
		t.Errorf("expected the session guard to reject the request, got %q", rec.Body.String())
	}
}

// A token for a user gets the catalogue.
func TestListWithATokenReturnsTheTemplates(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(Template{ID: uuid.New(), UserID: owner, Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true})
	router := newTestRouter(t, repo, owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var envelope types.Envelope[ListResponse]
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode the response: %v", err)
	}

	if envelope.Data.Total != 1 {
		t.Fatalf("expected 1 template, got %d", envelope.Data.Total)
	}

	if envelope.Data.Data[0].Name != "shared" {
		t.Errorf("expected the template %q, got %q", "shared", envelope.Data.Data[0].Name)
	}
}

// The catalogue can be filtered by kind, which is how a user finds the
// templates for the terminal they use.
func TestListFiltersByKind(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(
		Template{ID: uuid.New(), UserID: owner, Name: "opencode one", Slug: "opencode-one", Kind: KindOpencode, IsPublic: true},
		Template{ID: uuid.New(), UserID: owner, Name: "codex one", Slug: "codex-one", Kind: KindCodex, IsPublic: true},
	)
	router := newTestRouter(t, repo, owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/?kind=codex", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var envelope types.Envelope[ListResponse]
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode the response: %v", err)
	}

	if envelope.Data.Total != 1 {
		t.Fatalf("expected 1 template, got %d", envelope.Data.Total)
	}

	if envelope.Data.Data[0].Name != "codex one" {
		t.Errorf("expected the codex template, got %q", envelope.Data.Data[0].Name)
	}
}

// A template can be published with a token.
func TestPublishWithATokenStoresTheTemplate(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	router := newTestRouter(t, repo, owner)

	body, err := json.Marshal(validRequest())
	if err != nil {
		t.Fatalf("failed to encode the request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/templates/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var envelope types.Envelope[TemplateDetail]
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode the response: %v", err)
	}

	if len(envelope.Data.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(envelope.Data.Files))
	}

	if len(envelope.Data.Variables) != 1 {
		t.Errorf("expected 1 variable, got %d", len(envelope.Data.Variables))
	}
}

// A template with a path that would escape the project is refused, and
// nothing is stored.
func TestPublishRefusesAPathThatEscapes(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	router := newTestRouter(t, repo, owner)

	request := validRequest()
	request.Files = []PublishFile{{Path: "../escape.md", Content: "x"}}

	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("failed to encode the request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/templates/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}

	if len(repo.createdIDs) != 0 {
		t.Errorf("expected nothing to be stored, got %d templates", len(repo.createdIDs))
	}
}

// A template id that is not an id is refused before the service is asked.
func TestGetWithAnInvalidIDIsRefused(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository()
	router := newTestRouter(t, repo, owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/not-an-id", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

// The archive download answers with a zip that holds the files of the
// template.
func TestArchiveDownloadServesAZip(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{ID: templateID, UserID: owner, Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true})
	repo.files[templateID] = []TemplateFile{
		{TemplateID: templateID, Path: "AGENTS.md", Content: "# hello"},
	}
	router := newTestRouter(t, repo, owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/"+templateID.String()+"/archive", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if contentType := rec.Header().Get("Content-Type"); contentType != "application/zip" {
		t.Errorf("expected the content type %q, got %q", "application/zip", contentType)
	}

	if !strings.Contains(rec.Body.String(), "AGENTS.md") {
		t.Errorf("expected the file name in the archive, got %q", rec.Body.String())
	}
}
