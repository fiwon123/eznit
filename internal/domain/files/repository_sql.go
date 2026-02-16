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

func (r *sqlRepository) StorageFile(file File) (MsgResponse, bool) {
	_, err := r.db.NamedExec("INSERT INTO files (name, ext, path) VALUES (:name, :ext, :path)", file)
	if err != nil {
		fmt.Println(err)
		return MsgResponse{Msg: "internal server error"}, false
	}

	return MsgResponse{
		Msg: "file storaged!",
	}, true
}

func (r *sqlRepository) DeleteFile(id int) (MsgResponse, bool) {
	_, err := r.db.NamedExec("DELETE FROM users WHERE id=$1", id)
	if err != nil {
		fmt.Println(err)
		return MsgResponse{Msg: "internal server error"}, false
	}

	return MsgResponse{
		Msg: "file deleted!",
	}, true

}
