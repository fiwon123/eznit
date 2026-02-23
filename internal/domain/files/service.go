package files

import (
	"fmt"
	"io"
	"log/slog"
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
	s.logger.Debug("GetFiles")

	files, ok := s.db.GetFiles()
	if !ok {
		s.logger.Error("Files not found!")
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
	s.logger.Debug("GetFilesForUser", slog.String("userID", userID))

	files, ok := s.db.GetFilesForUser(userID)
	if !ok {
		s.logger.Error("Files not found!")
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
	s.logger.Debug("GetFile", slog.String("id", id))

	file, ok := s.db.GetFile(id)
	if !ok {
		s.logger.Error("File not found!")
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
	s.logger.Debug("GetFileForUser", slog.String("id", id), slog.String("userID", userID))

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		s.logger.Error("File not found!")
		return nil, false
	}

	return file, true
}

func (s *service) StorageFile(file multipart.File, header *multipart.FileHeader, contentType string, userID string) (MsgResponse, bool) {
	s.logger.Debug("StorageFile", slog.String("userID", userID))

	err := helper.CreatePathIfNotExists(s.uploadFolder)
	if err != nil {
		s.logger.Error("storage file failed!")
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
		s.logger.Error("storage file failed!")
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	dst, err := os.Create(finalPath)
	if err != nil {
		s.logger.Error("create file path failed: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		s.logger.Error("copy file error: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	s.logger.Debug("File storaged!")

	return MsgResponse{
		Msg: "file storaged!",
	}, true
}

func (s *service) DeleteFile(id string) (MsgResponse, bool) {
	s.logger.Debug("DeleteFile ", slog.String("id", id))

	if id == "" {
		s.logger.Error("id is empty")
		return MsgResponse{
			Msg: "id is empty",
		}, false
	}

	file, ok := s.db.GetFile(id)
	if !ok {
		s.logger.Error("file not found")
		return MsgResponse{
			"file not found",
		}, false
	}

	err := os.RemoveAll(file.Path)
	if err != nil {
		s.logger.Error("filepath not exists: ", slog.Any("error", err))
		return MsgResponse{
			"filepath not exists",
		}, false
	}

	ok = s.db.DeleteFile(id)
	if !ok {
		s.logger.Error("can't delete file")
		return MsgResponse{
			"can't delete file",
		}, false
	}

	s.logger.Debug("File Deleted!")

	return MsgResponse{
		Msg: "file deleted!",
	}, true
}

func (s *service) DeleteFileForUser(id string, userID string) (MsgResponse, bool) {
	s.logger.Debug("DeleteFileForUser ", slog.String("id", id), slog.String("userID", userID))

	if id == "" {
		s.logger.Error("id is empty")
		return MsgResponse{
			Msg: "id is empty",
		}, false
	}

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		s.logger.Error("file not found")
		return MsgResponse{
			"file not found",
		}, false
	}

	err := os.RemoveAll(file.Path)
	if err != nil {
		s.logger.Error("filepath not exists: ", slog.Any("error", err))
		return MsgResponse{
			"filepath not exists",
		}, false
	}

	ok = s.db.DeleteFileForUser(id, userID)
	if !ok {
		s.logger.Error("can't delete file")
		return MsgResponse{
			"can't delete file",
		}, false
	}

	s.logger.Debug("File Deleted!")

	return MsgResponse{
		Msg: "file deleted!",
	}, true
}

func (s *service) UpdateFile(id string, file multipart.File, header *multipart.FileHeader) (MsgResponse, bool) {

	s.logger.Debug("UpdateFile", slog.String("id", id))

	err := helper.CreatePathIfNotExists(s.uploadFolder)
	if err != nil {
		s.logger.Error("create path failed: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	fileInfo, ok := s.db.GetFile(id)
	if !ok {
		s.logger.Error("file not found")
		return MsgResponse{
			Msg: "invalid id",
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
		s.logger.Error(" update file failed")
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	dst, err := os.Create(finalPath)
	if err != nil {
		s.logger.Error("create file failed: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		s.logger.Error("copy file failed: ", slog.Any("error", err))
		return MsgResponse{
			Msg: "internal server error",
		}, false
	}

	s.logger.Debug("file updated!")

	return MsgResponse{
		Msg: "file updated!",
	}, true
}
