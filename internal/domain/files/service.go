package files

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
)

type service struct {
	uploadFolder string
	db           Repository
	logger       *logger.Config
}

func NewService(db Repository, uploadFolder string, logger *logger.Config) *service {
	return &service{
		uploadFolder: uploadFolder,
		db:           db,
		logger:       logger,
	}
}

func (s *service) GetFiles() (ListResponse, bool) {
	fmt.Println("GetFiles")

	files, ok := s.db.GetFiles()
	if !ok {
		return ListResponse{}, false
	}

	var resp []FileData
	for _, file := range files {
		resp = append(resp, FileData{
			ID:      file.ID,
			Name:    file.Name,
			Ext:     file.Ext,
			Version: file.Version,
		})
	}

	return ListResponse{
		Data:  resp,
		Total: len(resp),
	}, true
}

func (s *service) GetFilesForUser(userID string) (ListResponse, bool) {
	files, ok := s.db.GetFilesForUser(userID)
	if !ok {
		return ListResponse{}, false
	}

	var resp []FileData
	for _, file := range files {
		resp = append(resp, FileData{
			ID:      file.ID,
			Name:    file.Name,
			Ext:     file.Ext,
			Version: file.Version,
		})
	}

	return ListResponse{
		Data:  resp,
		Total: len(resp),
	}, true
}

func (s *service) GetFile(id string) (SingleReponse, bool) {
	file, ok := s.db.GetFile(id)
	if !ok {
		return SingleReponse{}, false
	}

	return SingleReponse{
		Data: FileData{
			ID:   file.ID,
			Name: file.Name,
			Ext:  file.Ext,
		},
	}, true
}

func (s *service) GetFileForUser(id string, userID string) (*File, bool) {
	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		return nil, false
	}

	return file, true
}

func (s *service) StorageFile(file multipart.File, header *multipart.FileHeader, contentType string, userID string) (MsgResponse, bool) {

	err := helper.CreatePathIfNotExists(s.uploadFolder)
	if err != nil {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	fullname := filepath.Base(header.Filename)
	ext := filepath.Ext(fullname)

	cleanName := strings.ReplaceAll(fullname, ext, "")
	ext = strings.ReplaceAll(ext, ".", "")
	finalPath := filepath.Join(s.uploadFolder, fmt.Sprintf("%d_%s", time.Now().Unix(), cleanName))

	storageFile := File{
		UserID:      userID,
		Name:        cleanName,
		Ext:         ext,
		Path:        finalPath,
		ContentType: contentType,
	}

	ok := s.db.StorageFile(storageFile)
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
		Msg: "file storaged!",
	}, true
}

func (s *service) DeleteFile(id string) (MsgResponse, bool) {
	if id == "" {
		return MsgResponse{
			Msg: "id is empty",
		}, false
	}

	file, ok := s.db.GetFile(id)
	if !ok {
		return MsgResponse{
			"file not found",
		}, false
	}

	err := os.RemoveAll(file.Path)
	if err != nil {
		return MsgResponse{
			"filepath not exists",
		}, false
	}

	ok = s.db.DeleteFile(id)
	if !ok {
		return MsgResponse{
			"can't delete file",
		}, false
	}

	return MsgResponse{
		Msg: "file deleted!",
	}, true
}

func (s *service) DeleteFileForUser(id string, userID string) (MsgResponse, bool) {
	if id == "" {
		return MsgResponse{
			Msg: "id is empty",
		}, false
	}

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		return MsgResponse{
			"file not found",
		}, false
	}

	err := os.RemoveAll(file.Path)
	if err != nil {
		return MsgResponse{
			"filepath not exists",
		}, false
	}

	ok = s.db.DeleteFileForUser(id, userID)
	if !ok {
		return MsgResponse{
			"can't delete file",
		}, false
	}

	return MsgResponse{
		Msg: "file deleted!",
	}, true
}

func (s *service) UpdateFile(id string, file multipart.File, header *multipart.FileHeader) (MsgResponse, bool) {

	err := helper.CreatePathIfNotExists(s.uploadFolder)
	if err != nil {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	fileInfo, ok := s.db.GetFile(id)
	if !ok {
		return MsgResponse{
			Msg: "id is invalid",
		}, false
	}

	cleanName := filepath.Base(header.Filename)
	ext := filepath.Ext(cleanName)
	finalPath := filepath.Join(s.uploadFolder, fmt.Sprintf("%d_%s", time.Now().Unix(), cleanName))

	updateFile := File{
		ID:     id,
		UserID: fileInfo.UserID,
		Name:   cleanName,
		Ext:    ext,
		Path:   finalPath,
	}

	ok = s.db.UpdateFile(updateFile)
	if !ok {
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	dst, err := os.Create(finalPath)
	if err != nil {
		fmt.Println(err)
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		fmt.Println(err)
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	return MsgResponse{
		Msg: "file updated!",
	}, true
}
