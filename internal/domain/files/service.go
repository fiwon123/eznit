package files

import (
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fiwon123/eznit/pkg/errors"
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

func (s *service) GetFiles() (ListResponse, *errors.AppError) {
	s.logger.Debug("GetFiles")

	files, ok := s.db.GetFiles()
	if !ok {
		return ListResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to get failes")
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
	}, nil
}

func (s *service) GetFilesForUser(userID string) (ListResponse, *errors.AppError) {
	s.logger.Debug("GetFilesForUser", slog.String("userID", userID))

	files, ok := s.db.GetFilesForUser(userID)
	if !ok {
		return ListResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to get failes for user")
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
	}, nil
}

func (s *service) GetFile(id string) (FileData, *errors.AppError) {
	s.logger.Debug("GetFile", slog.String("id", id))

	file, ok := s.db.GetFile(id)
	if !ok {
		return FileData{}, errors.NewAppError(http.StatusInternalServerError, "failed to get file")
	}

	return FileData{
		ID:   file.ID,
		Name: file.Name,
		Ext:  file.Ext,
	}, nil
}

func (s *service) GetFileForUser(id string, userID string) (*File, *errors.AppError) {
	s.logger.Debug("GetFileForUser", slog.String("id", id), slog.String("userID", userID))

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get faile for user")
	}

	return file, nil
}

func (s *service) StorageFile(file multipart.File, header *multipart.FileHeader, contentType string, userID string) (string, *errors.AppError) {

	s.logger.Debug("StorageFile", slog.String("userID", userID))

	err := helper.CreatePathIfNotExists(s.uploadFolder)
	if err != nil {
		s.logger.Error("failed to create path", slog.Any("error", err.Error()))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
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
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}

	dst, err := os.Create(finalPath)
	if err != nil {
		s.logger.Error("create file path failed: ", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		s.logger.Error("copy file error: ", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}

	s.logger.Debug("File storaged!")

	return "file storaged!", nil
}

func (s *service) DeleteFile(id string) (string, *errors.AppError) {
	s.logger.Debug("DeleteFile ", slog.String("id", id))

	if id == "" {
		s.logger.Warn("id is empty")
		return "", errors.NewAppError(http.StatusBadRequest, "id is empty")
	}

	file, ok := s.db.GetFile(id)
	if !ok {
		s.logger.Warn("file not found")
		return "", errors.NewAppError(http.StatusBadRequest, "file not found")
	}

	err := os.RemoveAll(file.Path)
	if err != nil {
		s.logger.Warn("filepath not exists, but will continue to delete", slog.Any("error", err))
	}

	ok = s.db.DeleteFile(id)
	if !ok {
		s.logger.Error("can't delete file")
		return "", errors.NewAppError(http.StatusInternalServerError, "can't delete file")
	}

	s.logger.Debug("File Deleted!")

	return "file deleted!", nil
}

func (s *service) DeleteFileForUser(id string, userID string) (string, *errors.AppError) {
	s.logger.Debug("DeleteFileForUser ", slog.String("id", id), slog.String("userID", userID))

	if id == "" {
		s.logger.Warn("id is empty")
		return "", errors.NewAppError(http.StatusBadRequest, "id is empty")
	}

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		s.logger.Warn("file not found")
		return "", errors.NewAppError(http.StatusBadRequest, "file not found")
	}

	err := os.RemoveAll(file.Path)
	if err != nil {
		s.logger.Warn("filepath not exists, but will continue to delete", slog.Any("error", err))
	}

	ok = s.db.DeleteFileForUser(id, userID)
	if !ok {
		s.logger.Error("can't delete file")
		return "", errors.NewAppError(http.StatusInternalServerError, "can't delete file")
	}

	s.logger.Debug("File Deleted!")

	return "file deleted!", nil
}

func (s *service) UpdateFile(id string, file multipart.File, header *multipart.FileHeader) (string, *errors.AppError) {

	s.logger.Debug("UpdateFile", slog.String("id", id))

	fileInfo, ok := s.db.GetFile(id)
	if !ok {
		s.logger.Warn("file not found")
		return "", errors.NewAppError(http.StatusBadRequest, "file not found")
	}

	cleanName := filepath.Base(header.Filename)
	ext := filepath.Ext(cleanName)
	finalPath := filepath.Join(s.uploadFolder, fmt.Sprintf("%d_%s", time.Now().Unix(), cleanName))

	updateFile := File{
		ID:      id,
		UserID:  fileInfo.UserID,
		Name:    cleanName,
		Ext:     ext,
		Version: fileInfo.Version + 1,
		Path:    finalPath,
	}

	err := helper.CreatePathIfNotExists(s.uploadFolder)
	if err != nil {
		s.logger.Error("create path failed", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}

	dest, err := os.Create(finalPath)
	if err != nil {
		s.logger.Error("create file failed: ", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}
	defer dest.Close()

	_, err = io.Copy(dest, file)
	if err != nil {
		s.logger.Error("copy file failed: ", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}

	ok = s.db.UpdateFile(updateFile)
	if !ok {
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}

	s.logger.Debug("file updated!")

	return "file updated!", nil
}
