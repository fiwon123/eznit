package files

import (
	"context"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/google/uuid"
)

// A filename carrying a path is refused instead of being stripped: the
// client is told, and nothing is written.
func TestStorageFileRejectsAFilenameWithAPath(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()
	svc := newTestService(t, repo, t.TempDir())

	content := []byte("payload")
	header := &multipart.FileHeader{Filename: "../../etc/passwd"}

	_, appError := svc.StorageFile(userContext(owner), multipartFile(t, content), header, "text/plain")
	if appError == nil {
		t.Fatal("expected the upload to be refused")
	}

	if appError.StatusCode() != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
	}

	if len(repo.stored) != 0 {
		t.Errorf("expected nothing to be stored, got %d files", len(repo.stored))
	}
}

// A Windows-style separator is refused the same way.
func TestStorageFileRejectsAFilenameWithABackslash(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()
	svc := newTestService(t, repo, t.TempDir())

	header := &multipart.FileHeader{Filename: `..\..\windows\system32\config`}

	_, appError := svc.StorageFile(userContext(owner), multipartFile(t, []byte("payload")), header, "text/plain")
	if appError == nil {
		t.Fatal("expected the upload to be refused")
	}

	if appError.StatusCode() != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
	}

	if len(repo.stored) != 0 {
		t.Errorf("expected nothing to be stored, got %d files", len(repo.stored))
	}
}

// A name that is a directory reference on its own is refused.
func TestStorageFileRejectsADirectoryName(t *testing.T) {
	owner := uuid.New()
	repo := newFakeRepository()
	svc := newTestService(t, repo, t.TempDir())

	header := &multipart.FileHeader{Filename: ".."}

	_, appError := svc.StorageFile(userContext(owner), multipartFile(t, []byte("payload")), header, "text/plain")
	if appError == nil {
		t.Fatal("expected the upload to be refused")
	}

	if appError.StatusCode() != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
	}
}

// A plain name is accepted, and the stored path is inside the upload root —
// checked, not assumed.
func TestStorageFileKeepsThePathInsideTheUploadRoot(t *testing.T) {
	owner := uuid.New()
	uploadRoot := t.TempDir()

	repo := newFakeRepository()
	svc := newTestService(t, repo, uploadRoot)

	header := &multipart.FileHeader{Filename: "report.txt"}

	_, appError := svc.StorageFile(userContext(owner), multipartFile(t, []byte("payload")), header, "text/plain")
	if appError != nil {
		t.Fatalf("expected the upload to be accepted, got error %v", appError)
	}

	if len(repo.stored) != 1 {
		t.Fatalf("expected one stored file, got %d", len(repo.stored))
	}

	stored := repo.stored[0]

	if !helper.PathIsInside(uploadRoot, stored.Path) {
		t.Errorf("the stored path %q is outside the upload root %q", stored.Path, uploadRoot)
	}

	if stored.Name != "report" || stored.Ext != "txt" {
		t.Errorf("expected the name to be kept, got name %q and ext %q", stored.Name, stored.Ext)
	}
}

// The update route builds a path from the client filename too, so it
// refuses the same names.
func TestUpdateFileRejectsAFilenameWithAPath(t *testing.T) {
	owner := uuid.New()
	fileID := uuid.New()

	repo := newFakeRepository(File{ID: fileID, UserID: owner, Name: "report", Ext: "txt", Version: 1})
	svc := newTestService(t, repo, t.TempDir())

	header := &multipart.FileHeader{Filename: "../../etc/passwd"}

	_, appError := svc.UpdateFile(userContext(owner), multipartFile(t, []byte("payload")), header, fileID, owner)
	if appError == nil {
		t.Fatal("expected the update to be refused")
	}

	if appError.StatusCode() != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, appError.StatusCode())
	}
}

// An update is a new version: the index row moves to it, and it gets its own
// history row. The version before it keeps its row — a history that is
// rewritten to the newest path cannot say where anything older went (#65).
func TestUpdateFileRecordsTheNewVersionInHistory(t *testing.T) {
	owner := uuid.New()
	fileID := uuid.New()

	firstVersion := File{
		ID:      fileID,
		UserID:  owner,
		Name:    "report",
		Ext:     "txt",
		Version: 1,
		Path:    filepath.Join(t.TempDir(), "1_report.txt"),
	}

	uploadRoot := t.TempDir()
	// The upload left the file's own folder behind, and an update writes the
	// next version into it.
	if err := os.MkdirAll(filepath.Join(uploadRoot, owner.String(), fileID.String()), 0o755); err != nil {
		t.Fatalf("make the file's folder: %v", err)
	}

	repo := newFakeRepository(firstVersion)
	svc := newTestService(t, repo, uploadRoot)

	header := &multipart.FileHeader{Filename: "report.txt"}

	if _, appError := svc.UpdateFile(userContext(owner), multipartFile(t, []byte("the second version")), header, fileID, owner); appError != nil {
		t.Fatalf("expected the update to succeed, got %v", appError)
	}

	if got := repo.files[fileID].Version; got != 2 {
		t.Errorf("the index row is version %d, want 2", got)
	}

	if len(repo.history) != 1 {
		t.Fatalf("expected one history row for the new version, got %d", len(repo.history))
	}

	history := repo.history[0]
	if history.ID != fileID || history.Version != 2 {
		t.Errorf("the history row is file %s version %d, want %s version 2", history.ID, history.Version, fileID)
	}

	if base := filepath.Base(history.Path); base != "2_report.txt" {
		t.Errorf("the history row points at %q, want 2_report.txt", base)
	}

	if repo.history[0].Path == firstVersion.Path {
		t.Error("the history row points at the first version's path")
	}
}

// The list is bounded: the service asks the repository for a limited list
// and returns no more than that.
func TestGetFilesIsBounded(t *testing.T) {
	owner := uuid.New()

	var files []File
	for i := 0; i < listLimit+25; i++ {
		files = append(files, File{ID: uuid.New(), UserID: owner, Name: "file"})
	}

	repo := newFakeRepository(files...)
	svc := newTestService(t, repo, t.TempDir())

	resp, appError := svc.GetFiles(userContext(owner))
	if appError != nil {
		t.Fatalf("expected the list to be returned, got error %v", appError)
	}

	if repo.lastLimit != listLimit {
		t.Errorf("expected the repository to be asked for %d files, got %d", listLimit, repo.lastLimit)
	}

	if len(resp.Data) > listLimit {
		t.Errorf("expected at most %d files, got %d", listLimit, len(resp.Data))
	}
}

// The list needs a session: without a user in the context the service
// refuses, so a route registered without the guard fails closed.
func TestGetFilesWithoutAUserIsUnauthorized(t *testing.T) {
	svc := newTestService(t, newFakeRepository(), t.TempDir())

	_, appError := svc.GetFiles(context.Background())
	if appError == nil {
		t.Fatal("expected the list to be refused")
	}

	if appError.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, appError.StatusCode())
	}
}

// userContext returns a context carrying the user the guard would set.
func userContext(userID uuid.UUID) context.Context {
	return context.WithValue(context.Background(), "user_id", userID)
}

// multipartFile returns a multipart.File holding the given content.
func multipartFile(t *testing.T, content []byte) multipart.File {
	t.Helper()

	path := filepath.Join(t.TempDir(), "upload")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("failed to write the test file: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open the test file: %v", err)
	}

	return file
}
