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

type Service struct {
	db Repository
}

func NewService(db Repository) *Service {
	return &Service{
		db: db,
	}
}

func (s *Service) GetFiles() ([]DataResponse, bool) {
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

func (s *Service) GetFile() ([]DataResponse, bool) {
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

func (s *Service) StorageFile(file multipart.File, header *multipart.FileHeader) (MsgResponse, bool) {

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
		UserID: 2,
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
