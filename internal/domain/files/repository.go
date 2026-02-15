package files

import (
	"time"
)

type File struct {
	ID        int       `db:"id"`
	UserID    int       `db:"id_user"`
	Name      string    `db:"name"`
	Ext       string    `db:"ext"`
	Path      string    `db:"path"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Repository interface {
	GetFiles() ([]File, bool)
	StorageFile(file File) (MsgResponse, bool)
}
