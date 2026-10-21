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
}

// newFakeRepository returns a repository holding the given files.
func newFakeRepository(files ...File) *fakeRepository {
	repo := &fakeRepository{files: map[uuid.UUID]File{}}
	for _, file := range files {
		repo.files[file.ID] = file
	}
	return repo
}

func (r *fakeRepository) GetFiles() ([]File, bool) {
	all := make([]File, 0, len(r.files))
	for _, file := range r.files {
		all = append(all, file)
	}
	return all, true
}

func (r *fakeRepository) GetFilesForUser(userID uuid.UUID) ([]File, bool) {
	var owned []File
	for _, file := range r.files {
		if file.UserID == userID {
			owned = append(owned, file)
		}
	}
	return owned, true
}

func (r *fakeRepository) GetFile(id uuid.UUID) (*File, bool) {
	file, ok := r.files[id]
	if !ok {
		return nil, false
	}
	return &file, true
}

func (r *fakeRepository) GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, bool) {
	file, ok := r.files[id]
	if !ok || file.UserID != userID {
		return nil, false
	}
	return &file, true
}

func (r *fakeRepository) StorageFile(file File) bool {
	r.files[file.ID] = file
	r.stored = append(r.stored, file)
	return true
}

func (r *fakeRepository) StorageFileHistory(file File) bool {
	return true
}

func (r *fakeRepository) DeleteFile(id uuid.UUID) bool {
	delete(r.files, id)
	return true
}

func (r *fakeRepository) DeleteFileForUser(id uuid.UUID, userID uuid.UUID) bool {
	file, ok := r.files[id]
	if !ok || file.UserID != userID {
		return false
	}
	delete(r.files, id)
	return true
}

func (r *fakeRepository) DeleteFileHistoryForUser(id uuid.UUID, userID uuid.UUID) bool {
	return true
}

func (r *fakeRepository) UpdateFile(file File) bool {
	r.files[file.ID] = file
	return true
}

func (r *fakeRepository) IsUserOwner(id uuid.UUID, userID uuid.UUID) bool {
	file, ok := r.files[id]
	return ok && file.UserID == userID
}
