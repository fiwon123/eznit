package files

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type sqlRepository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *sqlRepository {
	return &sqlRepository{
		db: db,
	}
}

func (r *sqlRepository) GetFiles() ([]File, bool) {
	var files []File

	err := r.db.Select(&files, "SELECT * FROM files")
	if err != nil {
		fmt.Println(err)
		return []File{}, false
	}

	return files, true
}

func (r *sqlRepository) GetFile(id string) (*File, bool) {
	var file File

	err := r.db.Get(&file, "SELECT * FROM files WHERE id=$1", id)
	if err != nil {
		fmt.Println(err)
		return nil, false
	}

	return &file, true
}

func (r *sqlRepository) GetFileForUser(id string, userID string) (*File, bool) {
	var file File

	query := `SELECT * FROM files
		      WHERE id=$1 AND user_id=$2`

	err := r.db.Get(&file, query, id, userID)
	if err != nil {
		fmt.Println(err)
		return nil, false
	}

	return &file, true
}

func (r *sqlRepository) StorageFile(file File) bool {
	_, err := r.db.NamedExec("INSERT INTO files (name, ext, path, content_type) VALUES (:name, :ext, :path, :content_type)", file)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}

func (r *sqlRepository) DeleteFile(id string) bool {
	_, err := r.db.Exec("DELETE FROM files WHERE id=$1", id)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true

}

func (r *sqlRepository) UpdateFile(file File) bool {
	exec := "UPDATE files SET name=:name, ext=:ext, path=:path, updated_at=NOW() WHERE id=:id"

	_, err := r.db.NamedExec(exec, file)
	if err != nil {
		fmt.Println(err)
		return false
	}

	return true
}
