package files

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
)

type service struct {
	db Repository
}

func NewService(db Repository) *service {
	return &service{
		db: db,
	}
}

func (s *service) GetFiles() ([]DataResponse, bool) {
	files, ok := s.db.GetFiles()
	if !ok {
		return []DataResponse{}, false
	}

	var resp []DataResponse
	for _, file := range files {
		resp = append(resp, DataResponse{
			ID:   file.ID,
			Name: file.Name,
			Ext:  file.Ext,
		})
	}

	return resp, true
}

func (s *service) GetFile(id string) (DataResponse, bool) {
	file, ok := s.db.GetFile(id)
	if !ok {
		return DataResponse{}, false
	}

	return DataResponse{
		ID:   file.ID,
		Name: file.Name,
		Ext:  file.Ext,
	}, true
}

func (s *service) StorageFile(file multipart.File, header *multipart.FileHeader) (MsgResponse, bool) {

	err := helper.CreatePathIfNotExists("./uploads")
	if err != nil {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	cleanName := filepath.Base(header.Filename)
	ext := filepath.Ext(cleanName)
	finalPath := filepath.Join("./uploads", fmt.Sprintf("%d_%s", time.Now().Unix(), cleanName))

	storageFile := File{
		UserID: "2",
		Name:   cleanName,
		Ext:    ext,
		Path:   finalPath,
	}

	resp, ok := s.db.StorageFile(storageFile)
	if !ok {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	dst, err := os.Create(finalPath)
	if err != nil {
		fmt.Println(err)
		return MsgResponse{
			Msg: "internal server erro",
		}, false
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		fmt.Println(err)
		return MsgResponse{
			Msg: "internal server erro",
		}, false
	}

	return MsgResponse{
		Msg: resp.Msg,
	}, true
}

func (s *service) DeleteFile(id string) (MsgResponse, bool) {
	if id == "" {
		return MsgResponse{
			Msg: "id is empty",
		}, false
	}

	resp, ok := s.db.DeleteFile(id)
	if !ok {
		return MsgResponse{
			"can't delete file",
		}, false
	}

	return resp, true
}
