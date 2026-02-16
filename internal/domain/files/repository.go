package files

import (
	"time"
)

type File struct {
	ID        string    `db:"id"`
	UserID    string    `db:"id_user"`
	Name      string    `db:"name"`
	Ext       string    `db:"ext"`
	Path      string    `db:"path"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Repository interface {
	GetFiles() ([]File, bool)
	GetFile(id string) (File, bool)
	StorageFile(file File) (MsgResponse, bool)
	DeleteFile(id string) (MsgResponse, bool)
	UpdateFile(file File) (MsgResponse, bool)
}
