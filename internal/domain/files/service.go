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

// Files Service is responsible to handle all incoming request from handler
type service struct {
	uploadFolder string
	db           Repository
	logger       *logger.Config
}

// Return a new Files Service
func NewService(db Repository, uploadFolder string, logger *logger.Config) *service {
	return &service{
		uploadFolder: uploadFolder,
		db:           db,
		logger:       logger,
	}
}

// userIDFromContext returns the user the guard put in the context. The
// guard is the only thing that sets it, so a missing value means the
// request never went through the guard — the route is registered without
// it. The caller is rejected instead of the request panicking on the type
// assertion, so a misregistered route fails closed.
func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value("user_id").(uuid.UUID)
	return userID, ok
}

// GetFilesForUser lists the caller's own files. Every list route on this
// resource answers with the caller's own files: a file belongs to the user
// who uploaded it, so there is no instance-wide list to ask for.
func (s *service) GetFilesForUser(ctx context.Context) (ListResponse, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return ListResponse{}, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

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

// Get only a single File for any user
func (s *service) GetFile(ctx context.Context, id uuid.UUID) (*File, *errors.AppError) {

	s.logger.Debug("GetFile", slog.String("id", id.String()))

	file, ok := s.db.GetFile(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get file")
	}

	return file, nil
}

// Get only a single File for user
func (s *service) GetFileForUser(ctx context.Context, id uuid.UUID) (*File, *errors.AppError) {

	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("GetFileForUser", slog.String("id", id.String()), slog.String("userID", userID.String()))

	file, ok := s.db.GetFileForUser(id, userID)
	if !ok {
		s.logger.Warn("file not found for user")
		// The file does not exist, or it belongs to somebody else. Both
		// answer the same way: a caller must not be able to tell an id
		// that exists from one that does not.
		return nil, errors.NewAppError(http.StatusNotFound, "file not found")
	}

	return file, nil
}

// safeFileName validates the name the client sent and returns it unchanged.
//
// The name ends up in the path of the stored file, so a name carrying a
// path separator is rejected instead of being stripped. Stripping it
// quietly turns a traversal attempt into a file with an unrelated name,
// stored without a word to the client about it — the write stays inside
// the upload folder either way, but a request that is refused is a request
// the client can see.
func safeFileName(name string) (string, *errors.AppError) {
	if name == "" {
		return "", errors.NewAppError(http.StatusBadRequest, "invalid file name")
	}

	if strings.ContainsAny(name, `/\`) {
		return "", errors.NewAppError(http.StatusBadRequest, "invalid file name")
	}

	// A name that is a directory reference on its own. Any other ".."
	// inside a name is part of the name, not a path.
	if name == "." || name == ".." {
		return "", errors.NewAppError(http.StatusBadRequest, "invalid file name")
	}

	if strings.ContainsRune(name, 0) {
		return "", errors.NewAppError(http.StatusBadRequest, "invalid file name")
	}

	return name, nil
}

// Storage a single file for user
func (s *service) StorageFile(ctx context.Context, file multipart.File, header *multipart.FileHeader, contentType string) (string, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return "", errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("StorageFile", slog.String("userID", userID.String()))

	fileName, appError := safeFileName(header.Filename)
	if appError != nil {
		return "", appError
	}

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

	// The name is validated and the path is built from it, so the write
	// lands inside the upload root. The check is here anyway: it is the
	// one place that can say so, and it costs nothing.
	if !helper.PathIsInside(s.uploadFolder, finalPath) {
		s.logger.Error("the storage path is outside the upload folder", slog.String("path", finalPath))
		return "", errors.NewAppError(http.StatusInternalServerError, "storage file failed!")
	}

	storageFile := File{
		ID:          fileID,
		Version:     version,
		UserID:      userID,
		Name:        cleanName,
		Ext:         ext,
		Path:        finalPath,
		ContentType: contentType,
	}

	ok = s.db.StorageFile(storageFile)
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

// Delete a single file for user
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

// Update a single file for user
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

	fileName, appError := safeFileName(header.Filename)
	if appError != nil {
		return "", appError
	}

	ext := filepath.Ext(fileName)

	cleanName := strings.ReplaceAll(fileName, ext, "")
	finalPath := filepath.Join(s.uploadFolder, fileInfo.UserID.String(), fileInfo.ID.String(), strconv.Itoa(newVersion)+"_"+fileName)

	if !helper.PathIsInside(s.uploadFolder, finalPath) {
		s.logger.Error("the storage path is outside the upload folder", slog.String("path", finalPath))
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}

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

	// The new version gets its own history row; the rows of the versions
	// before it are left alone. An update is a new version, not a rewrite of
	// the old one (#65).
	ok = s.db.StorageFileHistory(updateFile)
	if !ok {
		s.logger.Error("can't store the file history")
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}

	ok = s.db.UpdateFile(updateFile)
	if !ok {
		return "", errors.NewAppError(http.StatusInternalServerError, "update file failed")
	}

	s.logger.Debug("file updated!")

	return "file updated!", nil
}
