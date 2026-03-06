package files

import (
	"time"

	"github.com/google/uuid"
)

type File struct {
	ID          uuid.UUID `db:"id"`
	UserID      uuid.UUID `db:"user_id"`
	Name        string    `db:"name"`
	Ext         string    `db:"ext"`
	Path        string    `db:"path"`
	Version     int       `db:"version"`
	ContentType string    `db:"content_type"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type Repository interface {
	GetFiles() ([]File, bool)
	GetFilesForUser(userID uuid.UUID) ([]File, bool)
	GetFile(id uuid.UUID) (*File, bool)
	GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, bool)
	StorageFile(file File) bool
	StorageFileHistory(file File) bool
	DeleteFile(id uuid.UUID) bool
	DeleteFileForUser(id uuid.UUID, userID uuid.UUID) bool
	UpdateFile(file File) bool
}
