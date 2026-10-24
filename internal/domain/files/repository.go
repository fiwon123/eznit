package files

import (
	"time"

	"github.com/google/uuid"
)

// File Model
type File struct {
	ID          uuid.UUID `db:"id"`
	UserID      uuid.UUID `db:"user_id"`
	Name        string    `db:"name"`
	Ext         string    `db:"ext"`
	Path        string    `db:"path"`
	Version     int       `db:"version"`
	ContentType string    `db:"content_type"`
	IsPublic    bool      `db:"is_public"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

// Files repository interface
type Repository interface {
	GetFilesForUser(userID uuid.UUID) ([]File, bool)
	GetFile(id uuid.UUID) (*File, bool)
	GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, bool)
	GetSharedFile(id uuid.UUID) (*File, bool)
	ListSharedFiles() ([]SharedFileWithOwner, bool)
	StorageFile(file File) bool
	StorageFileHistory(file File) bool
	DeleteFile(id uuid.UUID) bool
	DeleteFileForUser(id uuid.UUID, userID uuid.UUID) bool
	DeleteFileHistoryForUser(file uuid.UUID, userID uuid.UUID) bool
	UpdateFile(file File) bool
	SetFilePublic(id uuid.UUID, userID uuid.UUID, isPublic bool) bool
	IsUserOwner(id uuid.UUID, userID uuid.UUID) bool
}

// SharedFileWithOwner is a shared file joined with its owner's email for the
// catalogue view.
type SharedFileWithOwner struct {
	File
	OwnerEmail string `db:"owner_email"`
}
