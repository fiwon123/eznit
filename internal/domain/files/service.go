package files

import (
	"context"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
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

func (s *service) GetFiles(ctx context.Context) (ListResponse, *errors.AppError) {
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

func (s *service) GetFilesForUser(ctx context.Context) (ListResponse, *errors.AppError) {
	userID := ctx.Value("user_id").(uuid.UUID)
	s.logger.Debug("GetFilesForUser", slog.String("userID", userID.String()))

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

func (s *service) GetFile(ctx context.Context, id uuid.UUID) (*File, *errors.AppError) {

	s.logger.Debug("GetFile", slog.String("id", id.String()))

	file, ok := s.db.GetFile(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get file")
	}

	return file, nil
}

func (s *service) GetFileForUser(ctx context.Context, id uuid.UUID) (*File, *errors.AppError) {

	userID := ctx.Value("user_id").(uuid.UUID)
	s.logger.Debug("GetFileForUser", slog.String("id", id.String()), slog.String("userID", userID.String()))

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get faile for user")
	}

	return file, nil
}

func (s *service) StorageFile(ctx context.Context, file multipart.File, header *multipart.FileHeader, contentType string) (string, *errors.AppError) {
	userID := ctx.Value("user_id").(uuid.UUID)
	s.logger.Debug("StorageFile", slog.String("userID", userID.String()))

	fileName := filepath.Base(header.Filename)
	ext := filepath.Ext(fileName)

	cleanName := strings.ReplaceAll(fileName, ext, "")
	ext = strings.ReplaceAll(ext, ".", "")
	fileID, err := uuid.NewV7()

	s.logger.Debug("clean name", slog.String("name", cleanName))

	if err != nil {
		s.logger.Error("failed to generate uuid", slog.Any("error", err.Error()))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}
	version := 1
	finalPath := filepath.Join(s.uploadFolder, userID.String(), fileID.String(), strconv.Itoa(version)+"_"+fileName)

	storageFile := File{
		ID:          fileID,
		Version:     version,
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

	ok = s.db.StorageFileHistory(storageFile)
	if !ok {
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}

	err = helper.CreatePathIfNotExists(filepath.Dir(finalPath))
	if err != nil {
		s.logger.Error("failed to create path", slog.Any("error", err.Error()))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}

	dest, err := os.Create(finalPath)
	if err != nil {
		s.logger.Error("create file path failed: ", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}
	defer dest.Close()

	_, err = io.Copy(dest, file)
	if err != nil {
		s.logger.Error("copy file error: ", slog.Any("error", err))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}

	s.logger.Debug("File storaged!")

	return "file storaged!", nil
}

func (s *service) DeleteFileForUser(ctx context.Context, id uuid.UUID, userID uuid.UUID) (string, *errors.AppError) {

	s.logger.Debug("DeleteFileForUser ", slog.String("id", id.String()), slog.String("userID", userID.String()))

	if err := uuid.Validate(id.String()); err != nil {
		s.logger.Warn("invalid id", slog.String("error", err.Error()))
		return "", errors.NewAppError(http.StatusBadRequest, "invalid id")
	}

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		s.logger.Warn("file not found")
		return "", errors.NewAppError(http.StatusBadRequest, "file not found")
	}

	err := os.RemoveAll(filepath.Dir(file.Path))
	if err != nil {
		s.logger.Warn("filepath not exists, but will continue to delete", slog.Any("error", err))
	}

	ok = s.db.DeleteFileHistoryForUser(id, userID)
	if !ok {
		s.logger.Error("can't delete file history")
		return "", errors.NewAppError(http.StatusInternalServerError, "can't delete file")
	}

	ok = s.db.DeleteFileForUser(id, userID)
	if !ok {
		s.logger.Error("can't delete file")
		return "", errors.NewAppError(http.StatusInternalServerError, "can't delete file")
	}

	s.logger.Debug("File Deleted!")

	return "file deleted!", nil
}

func (s *service) UpdateFile(ctx context.Context, file multipart.File, header *multipart.FileHeader, id uuid.UUID, userID uuid.UUID) (string, *errors.AppError) {
	s.logger.Debug("UpdateFile", slog.String("id", id.String()))

	if !s.db.IsUserOwner(id, userID) {
		s.logger.Warn("not owner")
		return "", errors.NewAppError(http.StatusConflict, "not owner")
	}

	fileInfo, ok := s.db.GetFile(id)
	if !ok {
		s.logger.Warn("file not found")
		return "", errors.NewAppError(http.StatusBadRequest, "file not found")
	}

	newVersion := fileInfo.Version + 1

	fileName := filepath.Base(header.Filename)
	ext := filepath.Ext(fileName)

	cleanName := strings.ReplaceAll(fileName, ext, "")
	finalPath := filepath.Join(s.uploadFolder, fileInfo.UserID.String(), fileInfo.ID.String(), strconv.Itoa(newVersion)+"_"+fileName)

	updateFile := File{
		ID:      id,
		UserID:  fileInfo.UserID,
		Name:    cleanName,
		Ext:     ext,
		Version: newVersion,
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
