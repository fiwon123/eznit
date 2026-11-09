//go:build integration

package files

import (
	"errors"
	"testing"

	"github.com/fiwon123/eznit/internal/platform/dbtest"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

func newFiles(t *testing.T) (*sqlRepository, *sqlx.DB) {
	t.Helper()

	db := dbtest.Open(t)
	dbtest.Reset(t, db)

	return NewRepository(db, dbtest.Logger(t)), db
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

// storeFile writes the index row and its first history row, the way the upload
// path does.
func storeFile(t *testing.T, repo *sqlRepository, userID uuid.UUID, name string) File {
	t.Helper()

	file := File{
		ID:          uuid.New(),
		UserID:      userID,
		Name:        name,
		Ext:         "txt",
		Path:        userID.String() + "/" + name,
		ContentType: "text/plain",
	}

	if err := repo.StorageFile(file); err != nil {
		t.Fatalf("StorageFile(%s): %v", name, err)
	}
	if err := repo.StorageFileHistory(file); err != nil {
		t.Fatalf("StorageFileHistory(%s): %v", name, err)
	}
	return file
}

func TestFilesAreOwnerScoped(t *testing.T) {
	repo, db := newFiles(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	aliceFile := storeFile(t, repo, alice, "alice.txt")
	bobFile := storeFile(t, repo, bob, "bob.txt")

	got, err := repo.GetFileForUser(aliceFile.ID, alice)
	if err != nil {
		t.Fatalf("GetFileForUser(owner) error = %v", err)
	}
	if got.Name != "alice.txt" || got.Version != 1 {
		t.Errorf("got %+v, want alice.txt at version 1", got)
	}

	if _, err := repo.GetFileForUser(aliceFile.ID, bob); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFileForUser(other account) error = %v, want ErrNotFound", err)
	}

	aliceFiles, err := repo.GetFilesForUser(alice)
	if err != nil {
		t.Fatalf("GetFilesForUser(alice) error = %v", err)
	}
	if len(aliceFiles) != 1 || aliceFiles[0].ID != aliceFile.ID {
		t.Errorf("GetFilesForUser(alice) = %v, want only alice.txt", aliceFiles)
	}

	bobFiles, err := repo.GetFilesForUser(bob)
	if err != nil {
		t.Fatalf("GetFilesForUser(bob) error = %v", err)
	}
	if len(bobFiles) != 1 || bobFiles[0].ID != bobFile.ID {
		t.Errorf("GetFilesForUser(bob) = %v, want only bob.txt", bobFiles)
	}

	if owner, err := repo.IsUserOwner(aliceFile.ID, alice); err != nil || !owner {
		t.Errorf("IsUserOwner(alice) = %v, %v, want true, nil", owner, err)
	}
	if owner, err := repo.IsUserOwner(aliceFile.ID, bob); err != nil || owner {
		t.Errorf("IsUserOwner(bob) = %v, %v, want false, nil", owner, err)
	}

	if _, err := repo.GetFile(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFile(missing) error = %v, want ErrNotFound", err)
	}
}

// Sharing is a column, and only the owner's write may set it; the shared read
// joins the owner's email for the catalogue.
func TestShareFile(t *testing.T) {
	repo, db := newFiles(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	file := storeFile(t, repo, alice, "shared.txt")

	if _, err := repo.GetSharedFile(file.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSharedFile(private) error = %v, want ErrNotFound", err)
	}

	if err := repo.SetFilePublic(file.ID, alice, true); err != nil {
		t.Fatalf("SetFilePublic(owner) error = %v", err)
	}

	shared, err := repo.GetSharedFile(file.ID)
	if err != nil {
		t.Fatalf("GetSharedFile() error = %v", err)
	}
	if !shared.IsPublic {
		t.Error("is_public = false after SetFilePublic(true)")
	}

	listed, err := repo.ListSharedFiles()
	if err != nil {
		t.Fatalf("ListSharedFiles() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListSharedFiles() returned %d files, want 1", len(listed))
	}
	if listed[0].OwnerEmail != "alice@example.com" {
		t.Errorf("owner_email = %q, want alice@example.com", listed[0].OwnerEmail)
	}

	if err := repo.SetFilePublic(file.ID, bob, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetFilePublic(other account) error = %v, want ErrNotFound", err)
	}

	if err := repo.SetFilePublic(file.ID, alice, false); err != nil {
		t.Fatalf("SetFilePublic(false) error = %v", err)
	}
	if _, err := repo.GetSharedFile(file.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSharedFile() after unsharing error = %v, want ErrNotFound", err)
	}
}

// Every upload keeps its old version in files_history, and deleting the file
// removes the history with it (#65, #144).
func TestFileVersionsAndDelete(t *testing.T) {
	repo, db := newFiles(t)

	alice := createAccount(t, db, "alice@example.com")
	bob := createAccount(t, db, "bob@example.com")

	file := storeFile(t, repo, alice, "report.txt")

	file.Path = alice.String() + "/report.v2.txt"
	file.Version = 2
	if err := repo.UpdateFile(file); err != nil {
		t.Fatalf("UpdateFile() error = %v", err)
	}
	if err := repo.StorageFileHistory(file); err != nil {
		t.Fatalf("StorageFileHistory(v2) error = %v", err)
	}

	got, err := repo.GetFile(file.ID)
	if err != nil {
		t.Fatalf("GetFile() error = %v", err)
	}
	if got.Version != 2 || got.Path != file.Path {
		t.Errorf("got version %d at %q, want version 2 at %q", got.Version, got.Path, file.Path)
	}

	if n := dbtest.Count(t, db, "files_history", "file_id = $1", file.ID); n != 2 {
		t.Errorf("files_history rows = %d, want 2 (one per stored version)", n)
	}

	if err := repo.DeleteFileForUser(file.ID, bob); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteFileForUser(other account) error = %v, want ErrNotFound", err)
	}

	if err := repo.DeleteFileForUser(file.ID, alice); err != nil {
		t.Fatalf("DeleteFileForUser(owner) error = %v", err)
	}
	if _, err := repo.GetFile(file.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetFile() after delete error = %v, want ErrNotFound", err)
	}

	// The history cascade from 000014, not the service's cleanup.
	if n := dbtest.Count(t, db, "files_history", "file_id = $1", file.ID); n != 0 {
		t.Errorf("files_history rows after the delete = %d, want 0", n)
	}

	if err := repo.DeleteFile(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteFile(missing) error = %v, want ErrNotFound", err)
	}
}
