package files

import (
	"github.com/google/uuid"
)

// fakeRepository is an in-memory Repository. The rules under test — who may
// read a file, and where a write lands — live in the service, so the tests
// drive the service with a repository that needs no database.
type fakeRepository struct {
	files  map[uuid.UUID]File
	stored []File
	// history holds the rows StorageFileHistory was asked for, so a test can
	// assert an update adds a version instead of rewriting the ones before it.
	history []File
}

// newFakeRepository returns a repository holding the given files.
func newFakeRepository(files ...File) *fakeRepository {
	repo := &fakeRepository{files: map[uuid.UUID]File{}}
	for _, file := range files {
		repo.files[file.ID] = file
	}
	return repo
}

func (r *fakeRepository) GetFilesForUser(userID uuid.UUID) ([]File, error) {
	var owned []File
	for _, file := range r.files {
		if file.UserID == userID {
			owned = append(owned, file)
		}
	}
	return owned, nil
}

func (r *fakeRepository) GetFile(id uuid.UUID) (*File, error) {
	file, ok := r.files[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &file, nil
}

func (r *fakeRepository) GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, error) {
	file, ok := r.files[id]
	if !ok || file.UserID != userID {
		return nil, ErrNotFound
	}
	return &file, nil
}

func (r *fakeRepository) StorageFile(file File) error {
	r.files[file.ID] = file
	r.stored = append(r.stored, file)
	return nil
}

func (r *fakeRepository) StorageFileHistory(file File) error {
	r.history = append(r.history, file)
	return nil
}

func (r *fakeRepository) DeleteFile(id uuid.UUID) error {
	delete(r.files, id)
	return nil
}

func (r *fakeRepository) DeleteFileForUser(id uuid.UUID, userID uuid.UUID) error {
	file, ok := r.files[id]
	if !ok || file.UserID != userID {
		return ErrNotFound
	}
	delete(r.files, id)
	return nil
}

func (r *fakeRepository) DeleteFileHistoryForUser(id uuid.UUID, userID uuid.UUID) error {
	return nil
}

func (r *fakeRepository) UpdateFile(file File) error {
	r.files[file.ID] = file
	return nil
}

func (r *fakeRepository) GetSharedFile(id uuid.UUID) (*File, error) {
	file, ok := r.files[id]
	if !ok || !file.IsPublic {
		return nil, ErrNotFound
	}
	return &file, nil
}

func (r *fakeRepository) ListSharedFiles() ([]SharedFileWithOwner, error) {
	var out []SharedFileWithOwner
	for _, file := range r.files {
		if file.IsPublic {
			out = append(out, SharedFileWithOwner{File: file, OwnerEmail: "user@example.com"})
		}
	}
	return out, nil
}

func (r *fakeRepository) SetFilePublic(id uuid.UUID, userID uuid.UUID, isPublic bool) error {
	file, ok := r.files[id]
	if !ok || file.UserID != userID {
		return ErrNotFound
	}
	file.IsPublic = isPublic
	r.files[id] = file
	return nil
}

func (r *fakeRepository) IsUserOwner(id uuid.UUID, userID uuid.UUID) (bool, error) {
	file, ok := r.files[id]
	return ok && file.UserID == userID, nil
}
