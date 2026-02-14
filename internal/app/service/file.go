package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/internal/app/dto"
	"github.com/fiwon123/eznit/internal/domain/model"
	"github.com/fiwon123/eznit/pkg/helper"
)

func (config *Config) GetFiles() ([]dto.FileDataResponse, bool) {
	files, ok := config.db.GetFiles()
	if !ok {
		return []dto.FileDataResponse{}, false
	}

	var resp []dto.FileDataResponse
	for _, file := range files {
		resp = append(resp, dto.FileDataResponse{
			Name: file.Name,
			Ext:  file.Ext,
		})
	}

	return resp, true
}

func (config *Config) StorageFile(file multipart.File, header *multipart.FileHeader) (dto.FileMsgResponse, bool) {

	err := helper.CreatePathIfNotExists("./uploads")
	if err != nil {
		return dto.FileMsgResponse{
			Msg: "internal server error",
		}, false
	}

	cleanName := filepath.Base(header.Filename)
	ext := filepath.Ext(cleanName)
	finalPath := filepath.Join("./uploads", fmt.Sprintf("%d_%s", time.Now().Unix(), cleanName))

	storageFile := model.File{
		UserID: 2,
		Name:   cleanName,
		Ext:    ext,
		Path:   finalPath,
	}

	resp, ok := config.db.StorageFile(storageFile)
	if !ok {
		return dto.FileMsgResponse{
			Msg: "internal server error",
		}, false
	}

	dst, err := os.Create(finalPath)
	if err != nil {
		fmt.Println(err)
		return dto.FileMsgResponse{
			Msg: "internal server erro",
		}, false
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		fmt.Println(err)
		return dto.FileMsgResponse{
			Msg: "internal server erro",
		}, false
	}

	return dto.FileMsgResponse{
		Msg: resp.Msg,
	}, true
}
