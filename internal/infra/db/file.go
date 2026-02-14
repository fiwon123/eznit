package db

import (
	"fmt"

	"github.com/fiwon123/eznit/internal/app/dto"
	"github.com/fiwon123/eznit/internal/domain/model"
)

func (config *Config) GetFiles() ([]model.File, bool) {
	var files []model.File

	err := config.conn.Select(&files, "SELECT * FROM files")
	if err != nil {
		fmt.Println(err)
		return []model.File{}, false
	}

	return files, true
}

func (config *Config) StorageFile(file model.File) (dto.FileMsgResponse, bool) {
	_, err := config.conn.NamedExec("INSERT INTO files (name, ext, path) VALUES (:name, :ext, :path)", file)
	if err != nil {
		fmt.Println(err)
		return dto.FileMsgResponse{Msg: "internal server error"}, false
	}

	return dto.FileMsgResponse{
		Msg: "file storaged!",
	}, true
}
