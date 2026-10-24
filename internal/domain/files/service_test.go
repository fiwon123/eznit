package files

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// newTestService returns a service backed by the given repository and a
// logger that writes to a throwaway folder.
func newTestService(t *testing.T, repo Repository, uploadFolder string) *service {
	t.Helper()

	log, err := logger.NewConsole(t.TempDir(), false, true)
	if err != nil {
		t.Fatalf("failed to create the test logger: %v", err)
	}

	return NewService(repo, uploadFolder, log)
}

// A file that belongs to somebody else must not be readable through the
// service: the route resolves the file for the caller, so a token for user
// A cannot reach user B's file.
func TestGetFileForUserRejectsAnotherUsersFile(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()
	fileID := uuid.New()

	repo := newFakeRepository(File{ID: fileID, UserID: owner, Name: "secret", Ext: "txt"})
	svc := newTestService(t, repo, t.TempDir())

	ctx := context.WithValue(context.Background(), "user_id", stranger)

	file, appError := svc.GetFileForUser(ctx, fileID)
	if appError == nil {
		t.Fatalf("expected an error for a file owned by another user, got the file %+v", file)
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// A file that does not exist and a file owned by somebody else must be
// indistinguishable: a caller must not learn that an id exists.
func TestGetFileForUserReportsMissingFileAsNotFound(t *testing.T) {
	svc := newTestService(t, newFakeRepository(), t.TempDir())

	ctx := context.WithValue(context.Background(), "user_id", uuid.New())

	_, appError := svc.GetFileForUser(ctx, uuid.New())
	if appError == nil {
		t.Fatal("expected an error for a file that does not exist")
	}

	if appError.StatusCode() != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, appError.StatusCode())
	}
}

// The owner's own file is still readable.
func TestGetFileForUserReturnsTheOwnersFile(t *testing.T) {
	owner := uuid.New()
	fileID := uuid.New()

	repo := newFakeRepository(File{ID: fileID, UserID: owner, Name: "mine", Ext: "txt"})
	svc := newTestService(t, repo, t.TempDir())

	ctx := context.WithValue(context.Background(), "user_id", owner)

	file, appError := svc.GetFileForUser(ctx, fileID)
	if appError != nil {
		t.Fatalf("expected the file to be returned, got error %v", appError)
	}

	if file.ID != fileID {
		t.Errorf("expected file %s, got %s", fileID, file.ID)
	}
}

// The list is the caller's own files and nothing else: a file another user
// uploaded is not in it, however many rows the instance holds.
func TestGetFilesForUserListsOnlyTheCallersFiles(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	repo := newFakeRepository(
		File{ID: uuid.New(), UserID: owner, Name: "mine", Ext: "txt"},
		File{ID: uuid.New(), UserID: stranger, Name: "theirs", Ext: "txt"},
	)
	svc := newTestService(t, repo, t.TempDir())

	ctx := context.WithValue(context.Background(), "user_id", owner)

	resp, appError := svc.GetFilesForUser(ctx)
	if appError != nil {
		t.Fatalf("expected the list to be returned, got error %v", appError)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected the caller's one file, got %d files", len(resp.Data))
	}

	if resp.Data[0].Name != "mine" {
		t.Errorf("expected the caller's file, got %q", resp.Data[0].Name)
	}
}

// The list needs a session: without a user in the context the service
// refuses, so a route registered without the guard fails closed.
func TestGetFilesForUserWithoutAUserIsUnauthorized(t *testing.T) {
	svc := newTestService(t, newFakeRepository(), t.TempDir())

	_, appError := svc.GetFilesForUser(context.Background())
	if appError == nil {
		t.Fatal("expected the list to be refused")
	}

	if appError.StatusCode() != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, appError.StatusCode())
	}
}

// An empty list is still a list: a caller with no files gets `[]`, not `null`,
// so a client ranges over the payload instead of null-checking it. The wire
// shape is asserted, not the Go value — `null` is what the browser sees.
func TestGetFilesForUserAnswersAnEmptyListAsArray(t *testing.T) {
	svc := newTestService(t, newFakeRepository(), t.TempDir())

	resp, appError := svc.GetFilesForUser(userContext(uuid.New()))
	if appError != nil {
		t.Fatalf("expected an empty list, got error %v", appError)
	}

	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to encode the list: %v", err)
	}

	if got, want := string(body), `{"data":[],"total":0}`; got != want {
		t.Errorf("the empty list encodes as %s, want %s", got, want)
	}
}
