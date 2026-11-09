package files

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is what a read answers when there is no row (or the row is not
// visible to the caller), and what a write answers when it changed no row. It is
// distinct from a real failure so a caller can tell "no such file" from "the
// database is down".
var ErrNotFound = errors.New("not found")

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
//
// A read answers (value, error): ErrNotFound for absence, the query error for a
// real failure. A write answers error, ErrNotFound when it changed no row.
type Repository interface {
	GetFilesForUser(userID uuid.UUID) ([]File, error)
	GetFile(id uuid.UUID) (*File, error)
	GetFileForUser(id uuid.UUID, userID uuid.UUID) (*File, error)
	GetSharedFile(id uuid.UUID) (*File, error)
	ListSharedFiles() ([]SharedFileWithOwner, error)
	StorageFile(file File) error
	StorageFileHistory(file File) error
	DeleteFile(id uuid.UUID) error
	DeleteFileForUser(id uuid.UUID, userID uuid.UUID) error
	DeleteFileHistoryForUser(file uuid.UUID, userID uuid.UUID) error
	UpdateFile(file File) error
	SetFilePublic(id uuid.UUID, userID uuid.UUID, isPublic bool) error
	IsUserOwner(id uuid.UUID, userID uuid.UUID) (bool, error)
}

// SharedFileWithOwner is a shared file joined with its owner's email for the
// catalogue view.
type SharedFileWithOwner struct {
	File
	OwnerEmail string `db:"owner_email"`
}
