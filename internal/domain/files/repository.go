package files

import (
	"time"

	"github.com/oklog/ulid/v2"
)

type File struct {
	ID          ulid.ULID `db:"id"`
	UserID      ulid.ULID `db:"user_id"`
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
	GetFilesForUser(userID ulid.ULID) ([]File, bool)
	GetFile(id ulid.ULID) (*File, bool)
	GetFileForUser(id ulid.ULID, userID ulid.ULID) (*File, bool)
	StorageFile(file File) bool
	StorageFileHistory(file File) bool
	DeleteFile(id ulid.ULID) bool
	DeleteFileForUser(id ulid.ULID, userID ulid.ULID) bool
	UpdateFile(file File) bool
}
