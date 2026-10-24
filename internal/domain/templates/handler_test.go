package templates

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
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

	router, _ := newTestRouterWithRoot(t, repo, userID)

	return router
}

// newTestRouterWithRoot also returns the upload root, so a test can put the
// bytes of a binary file where the service reads them from.
func newTestRouterWithRoot(t *testing.T, repo Repository, userID uuid.UUID) (*chi.Mux, string) {
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

	root := t.TempDir()

	guard := middleware.NewGuard(sessions.NewService(newFakeSessionsRepository(session), log), log)
	handler := NewHandler(NewService(repo, root, log), guard, log)

	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	return router, root
}

// storeBinaryFile writes the bytes of a binary file under the upload root and
// returns the row that points at them.
func storeBinaryFile(t *testing.T, root string, templateID uuid.UUID, fileID uuid.UUID, path string, content []byte) TemplateFile {
	t.Helper()

	storagePath := "templates/" + templateID.String() + "/" + fileID.String() + "/" + filepath.Base(path)
	absolute := filepath.Join(root, filepath.FromSlash(storagePath))

	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		t.Fatalf("failed to create the storage folder: %v", err)
	}

	if err := os.WriteFile(absolute, content, 0o600); err != nil {
		t.Fatalf("failed to write the test file: %v", err)
	}

	return TemplateFile{
		ID:           fileID,
		TemplateID:   templateID,
		Path:         path,
		Kind:         FileKindBinary,
		Size:         int64(len(content)),
		StoragePath:  storagePath,
		IsExecutable: false,
	}
}

// The catalogue is public: without a token it answers with the templates
// their owners published, and with none of the private ones.
func TestListWithoutTokenReturnsOnlyThePublicTemplates(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(
		Template{ID: uuid.New(), UserID: owner, OwnerEmail: "owner@example.com", Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true},
		Template{ID: uuid.New(), UserID: owner, OwnerEmail: "owner@example.com", Name: "secret", Slug: "secret", Kind: KindGeneric},
	)
	router := newTestRouter(t, repo, owner)

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var envelope types.Envelope[ListResponse]
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode the response: %v", err)
	}

	if envelope.Data.Total != 1 {
		t.Fatalf("expected 1 public template, got %d: %s", envelope.Data.Total, rec.Body.String())
	}

	if name := envelope.Data.Data[0].Name; name != "shared" {
		t.Errorf("expected the public template %q, got %q", "shared", name)
	}

	// The address is what the catalogue shows for an account holder; a
	// visitor's catalogue leaves it out.
	if email := envelope.Data.Data[0].OwnerEmail; email != "" {
		t.Errorf("the owner's email %q is in a visitor's catalogue", email)
	}
}

// A public template's detail, archive and files answer without a token; a
// private template is the same 404 as an id that does not exist.
func TestPublicReadsWithoutAToken(t *testing.T) {
	owner := uuid.New()
	publicID := uuid.New()
	privateID := uuid.New()
	privateFileID := uuid.New()

	repo := newFakeRepository(
		Template{ID: publicID, UserID: owner, Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true},
		Template{ID: privateID, UserID: owner, Name: "secret", Slug: "secret", Kind: KindGeneric},
	)
	router, root := newTestRouterWithRoot(t, repo, owner)

	repo.files[publicID] = []TemplateFile{{TemplateID: publicID, Path: "AGENTS.md", Content: "# hello"}}
	repo.files[privateID] = []TemplateFile{storeBinaryFile(t, root, privateID, privateFileID, "secret.bin", []byte("secret"))}

	tests := []struct {
		name string
		path string
		want int
	}{
		{"public detail", "/v1/templates/" + publicID.String(), http.StatusOK},
		{"private detail", "/v1/templates/" + privateID.String(), http.StatusNotFound},
		{"missing detail", "/v1/templates/" + uuid.NewString(), http.StatusNotFound},
		{"public archive", "/v1/templates/" + publicID.String() + "/archive", http.StatusOK},
		{"private archive", "/v1/templates/" + privateID.String() + "/archive", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("expected status %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// The writes still need a session, and `mine` still needs one too: the public
// `{id}` route must not shadow it.
func TestWritesWithoutATokenAreRejected(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()

	repo := newFakeRepository(Template{ID: templateID, UserID: owner, Name: "mine", Slug: "mine", Kind: KindGeneric})
	router := newTestRouter(t, repo, owner)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"publish", http.MethodPost, "/v1/templates/"},
		{"mine", http.MethodGet, "/v1/templates/mine"},
		{"update", http.MethodPut, "/v1/templates/" + templateID.String()},
		{"delete", http.MethodDelete, "/v1/templates/" + templateID.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected status %d, got %d: %s", http.StatusUnauthorized, rec.Code, rec.Body.String())
			}

			if !strings.Contains(rec.Body.String(), "No token provided") {
				t.Errorf("expected the session guard to reject the request, got %q", rec.Body.String())
			}
		})
	}
}

// A token for a user gets the catalogue.
func TestListWithATokenReturnsTheTemplates(t *testing.T) {
	owner := uuid.New()

	repo := newFakeRepository(Template{ID: uuid.New(), UserID: owner, OwnerEmail: "owner@example.com", Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true})
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

	// A caller with a session still gets the attribution a visitor does not.
	if email := envelope.Data.Data[0].OwnerEmail; email != "owner@example.com" {
		t.Errorf("expected the owner's email %q, got %q", "owner@example.com", email)
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

// One file of a template is served with its bytes: a binary file comes from
// the disk, and the response is typed after the extension.
func TestTemplateFileContentServesABinaryFile(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()
	fileID := uuid.New()
	content := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

	repo := newFakeRepository(Template{ID: templateID, UserID: owner, Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true})
	router, root := newTestRouterWithRoot(t, repo, owner)
	repo.files[templateID] = []TemplateFile{storeBinaryFile(t, root, templateID, fileID, "assets/logo.png", content)}

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/"+templateID.String()+"/files/"+fileID.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	if contentType := rec.Header().Get("Content-Type"); contentType != "image/png" {
		t.Errorf("expected the content type %q, got %q", "image/png", contentType)
	}

	if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "logo.png") {
		t.Errorf("expected the file name in the disposition, got %q", disposition)
	}

	if !bytes.Equal(rec.Body.Bytes(), content) {
		t.Errorf("expected the bytes %v, got %v", content, rec.Body.Bytes())
	}
}

// A binary file of another user's private template is not found, not
// forbidden: its existence is not theirs to learn.
func TestTemplateFileContentHidesAnotherUsersPrivateTemplate(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	templateID := uuid.New()
	fileID := uuid.New()

	repo := newFakeRepository(Template{ID: templateID, UserID: owner, Name: "mine", Slug: "mine", Kind: KindGeneric})
	router, root := newTestRouterWithRoot(t, repo, stranger)
	repo.files[templateID] = []TemplateFile{storeBinaryFile(t, root, templateID, fileID, "secret.bin", []byte("secret"))}

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/"+templateID.String()+"/files/"+fileID.String()+"/content", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

// The archive carries the bytes of a binary file, not an empty entry.
func TestArchiveDownloadCarriesBinaryBytes(t *testing.T) {
	owner := uuid.New()
	templateID := uuid.New()
	fileID := uuid.New()
	content := []byte{0x00, 0x01, 0xfe, 0xff}

	repo := newFakeRepository(Template{ID: templateID, UserID: owner, Name: "shared", Slug: "shared", Kind: KindGeneric, IsPublic: true})
	router, root := newTestRouterWithRoot(t, repo, owner)
	repo.files[templateID] = []TemplateFile{
		{ID: uuid.New(), TemplateID: templateID, Path: "AGENTS.md", Kind: FileKindText, Content: "# hello"},
		storeBinaryFile(t, root, templateID, fileID, "assets/logo.bin", content),
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/templates/"+templateID.String()+"/archive", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	archive, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("failed to read the archive: %v", err)
	}

	if len(archive.File) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(archive.File))
	}

	entry, err := archive.Open("assets/logo.bin")
	if err != nil {
		t.Fatalf("expected the binary file in the archive: %v", err)
	}
	defer entry.Close()

	got, err := io.ReadAll(entry)
	if err != nil {
		t.Fatalf("failed to read the entry: %v", err)
	}

	if !bytes.Equal(got, content) {
		t.Errorf("expected the bytes %v, got %v", content, got)
	}
}
