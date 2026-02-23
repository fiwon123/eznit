package files

import (
	"time"
)

type File struct {
	ID          string    `db:"id"`
	UserID      string    `db:"user_id"`
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
	GetFilesForUser(userID string) ([]File, bool)
	GetFile(id string) (*File, bool)
	GetFileForUser(id string, userID string) (*File, bool)
	StorageFile(file File) bool
	DeleteFile(id string) bool
	DeleteFileForUser(id string, userID string) bool
	UpdateFile(file File) bool
}
