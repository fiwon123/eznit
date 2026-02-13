package service

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"time"

	"github.com/fiwon123/eznit/internal/app/dto"
)

func (config *Config) StorageFile(file multipart.File, header *multipart.FileHeader) (dto.FileMsgResponse, bool) {

	if err := os.MkdirAll("./uploads", 0755); err != nil {
		fmt.Println(err)
		return dto.FileMsgResponse{
			Msg: "internal server erro",
		}, false
	}

	cleanName := filepath.Base(header.Filename)

	finalPath := filepath.Join("./uploads", fmt.Sprintf("%d_%s", time.Now().Unix(), cleanName))

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
		Msg: "file uploded!",
	}, true
}
