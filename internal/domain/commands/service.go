package commands

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// service holds the rules of the command library: what a valid command is,
// whose it is, and where its bytes live.
type service struct {
	db     Repository
	logger *logger.Config
	// uploadFolder is the root a binary command is written under, the same
	// root the files and the templates use.
	uploadFolder string
}

// Return a new commands service
func NewService(db Repository, uploadFolder string, logger *logger.Config) *service {
	return &service{
		db:           db,
		uploadFolder: uploadFolder,
		logger:       logger,
	}
}

// The limits of a command. A text command is configuration and small; a
// binary command is a compiled helper and is allowed to be bigger.
const (
	maxNameLength     = 64
	maxTextBytes      = 256 * 1024
	maxBinaryBytes    = 10 * 1024 * 1024
	maxLibraryEntries = 500
)

// namePattern is the shape of a command name: letters, digits, dot, dash and
// underscore. It is a label, not a path — the target path lives on the
// template reference — and it goes into a storage filename, so a separator or
// a dot-dot is refused here.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// userIDFromContext reads the id the session guard put on the request.
func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value("user_id").(uuid.UUID)
	return userID, ok
}

// ListCommands returns the caller's library.
func (s *service) ListCommands(ctx context.Context) (ListResponse, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return ListResponse{}, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	stored, ok := s.db.ListCommands(userID)
	if !ok {
		return ListResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to list the commands")
	}

	data := make([]CommandData, 0, len(stored))
	for _, command := range stored {
		entry := CommandData{
			ID:           command.ID,
			Name:         command.Name,
			Kind:         command.Kind,
			Size:         command.Size,
			Content:      command.Content,
			IsExecutable: command.IsExecutable,
			CreatedAt:    command.CreatedAt,
			UpdatedAt:    command.UpdatedAt,
		}

		if command.Kind == CommandKindBinary {
			bytes, err := s.readStoredFile(command)
			if err != nil {
				s.logger.Error("failed to read a binary command", slog.Any("error", err))
				return ListResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to list the commands")
			}
			entry.ContentBase64 = base64.StdEncoding.EncodeToString(bytes)
		}

		data = append(data, entry)
	}

	return ListResponse{Data: data, Total: len(data)}, nil
}

// GetCommand returns one command of the caller's library.
func (s *service) GetCommand(ctx context.Context, id uuid.UUID) (*CommandData, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	command, ok := s.db.GetCommand(id, userID)
	if !ok {
		return nil, errors.NewAppError(http.StatusNotFound, "command not found")
	}

	data := CommandData{
		ID:           command.ID,
		Name:         command.Name,
		Kind:         command.Kind,
		Size:         command.Size,
		Content:      command.Content,
		IsExecutable: command.IsExecutable,
		CreatedAt:    command.CreatedAt,
		UpdatedAt:    command.UpdatedAt,
	}

	if command.Kind == CommandKindBinary {
		bytes, err := s.readStoredFile(*command)
		if err != nil {
			s.logger.Error("failed to read a binary command", slog.Any("error", err))
			return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get the command")
		}
		data.ContentBase64 = base64.StdEncoding.EncodeToString(bytes)
	}

	return &data, nil
}

// CreateCommand adds a command to the caller's library.
func (s *service) CreateCommand(ctx context.Context, request CommandRequest) (*CommandData, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	if s.libraryIsFull(userID) {
		return nil, errors.NewAppError(http.StatusConflict, "the command library is full")
	}

	command, appError := s.build(request, userID, uuid.Nil)
	if appError != nil {
		return nil, appError
	}

	if _, ok := s.db.GetCommandByName(userID, command.Name); ok {
		// The name is how a user refers to a command, so a duplicate is a
		// conflict, not a new entry.
		return nil, errors.NewAppError(http.StatusConflict, "a command with that name already exists")
	}

	id, err := uuid.NewV7()
	if err != nil {
		s.logger.Error("failed to generate the command id", slog.Any("error", err))
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to create the command")
	}

	command.ID = id
	command.StoragePath = storagePathOf(command)

	if appError := s.writeBinaryFile(command); appError != nil {
		return nil, appError
	}

	if !s.db.CreateCommand(command) {
		s.removeCommandStorage(command.ID)
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to create the command")
	}

	return s.GetCommand(ctx, command.ID)
}

// UpdateCommand replaces a command the caller owns.
func (s *service) UpdateCommand(ctx context.Context, id uuid.UUID, request CommandRequest) (*CommandData, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	existing, ok := s.db.GetCommand(id, userID)
	if !ok {
		return nil, errors.NewAppError(http.StatusNotFound, "command not found")
	}

	command, appError := s.build(request, userID, id)
	if appError != nil {
		return nil, appError
	}

	if command.Name != existing.Name {
		if other, ok := s.db.GetCommandByName(userID, command.Name); ok && other.ID != id {
			return nil, errors.NewAppError(http.StatusConflict, "a command with that name already exists")
		}
	}

	command.ID = id
	command.StoragePath = storagePathOf(command)

	if appError := s.writeBinaryFile(command); appError != nil {
		return nil, appError
	}

	if !s.db.UpdateCommand(command) {
		// The row still describes the old version. Only the bytes of the
		// new one are removed — the old bytes are what the row points at,
		// and they are not the new version's to delete.
		if command.StoragePath != existing.StoragePath {
			s.removeStoredFile(command)
		}
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to update the command")
	}

	// The bytes of the version being replaced are not referenced any more.
	if existing.StoragePath != "" && existing.StoragePath != command.StoragePath {
		s.removeStoredFile(*existing)
	}

	return s.GetCommand(ctx, id)
}

// DeleteCommand removes a command the caller owns, and its bytes.
func (s *service) DeleteCommand(ctx context.Context, id uuid.UUID) (string, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return "", errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	if _, ok := s.db.GetCommand(id, userID); !ok {
		return "", errors.NewAppError(http.StatusNotFound, "command not found")
	}

	// A template that pulls the command in keeps it: deleting it would
	// remove a file from a published template without its owner asking.
	if s.db.CommandInUse(id) {
		return "", errors.NewAppError(http.StatusConflict, "the command is used by a template")
	}

	if !s.db.DeleteCommand(id, userID) {
		return "", errors.NewAppError(http.StatusNotFound, "command not found")
	}

	s.removeCommandStorage(id)

	return "command deleted!", nil
}

// build validates a request and turns it into a row.
func (s *service) build(request CommandRequest, userID uuid.UUID, id uuid.UUID) (Command, *errors.AppError) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return Command{}, errors.NewAppError(http.StatusBadRequest, "a command needs a name")
	}

	if !namePattern.MatchString(name) {
		return Command{}, errors.NewAppError(http.StatusBadRequest, "invalid command name")
	}

	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		kind = CommandKindText
	}

	if !IsKind(kind) {
		return Command{}, errors.NewAppError(http.StatusBadRequest, "invalid command kind")
	}

	command := Command{
		ID:     id,
		UserID: userID,
		Name:   name,
		Kind:   kind,
		// A command is a thing you run unless the request says otherwise.
		IsExecutable: true,
	}

	if request.IsExecutable != nil {
		command.IsExecutable = *request.IsExecutable
	}

	if kind == CommandKindBinary {
		if request.Content != "" {
			return Command{}, errors.NewAppError(http.StatusBadRequest, "a binary command carries content_base64, not content")
		}

		bytes, appError := s.decodeBinary(request.ContentBase64)
		if appError != nil {
			return Command{}, appError
		}

		command.Bytes = bytes
		command.Size = int64(len(bytes))
	} else {
		if request.ContentBase64 != "" {
			return Command{}, errors.NewAppError(http.StatusBadRequest, "a text command carries content, not content_base64")
		}

		if len(request.Content) > maxTextBytes {
			return Command{}, errors.NewAppError(http.StatusBadRequest, "command too big")
		}

		command.Content = request.Content
		command.Size = int64(len(request.Content))
	}

	return command, nil
}

// decodeBinary turns the base64 body of a binary command into bytes, and
// refuses one that is too big before decoding it.
func (s *service) decodeBinary(encoded string) ([]byte, *errors.AppError) {
	if encoded == "" {
		return nil, errors.NewAppError(http.StatusBadRequest, "a binary command needs content_base64")
	}

	if int64(len(encoded)) > int64(maxBinaryBytes)*4/3+4 {
		return nil, errors.NewAppError(http.StatusBadRequest, "command too big")
	}

	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.NewAppError(http.StatusBadRequest, "invalid base64 content in the command")
	}

	if len(bytes) > maxBinaryBytes {
		return nil, errors.NewAppError(http.StatusBadRequest, "command too big")
	}

	return bytes, nil
}

// storagePathOf is where the bytes of a binary command live, relative to the
// upload root, with forward slashes so the row reads the same on every
// machine.
func storagePathOf(command Command) string {
	return "commands/" + command.ID.String() + "/" + command.Name
}

// absoluteStoragePath is the location a row points at, on this machine.
func (s *service) absoluteStoragePath(storagePath string) string {
	return filepath.Join(s.uploadFolder, filepath.FromSlash(storagePath))
}

// commandStorageDir is the directory of one command's bytes.
func (s *service) commandStorageDir(id uuid.UUID) string {
	return filepath.Join(s.uploadFolder, "commands", id.String())
}

// writeBinaryFile writes the bytes of a binary command. A text command has
// none to write.
func (s *service) writeBinaryFile(command Command) *errors.AppError {
	if command.Kind != CommandKindBinary {
		return nil
	}

	absolute := s.absoluteStoragePath(command.StoragePath)
	if !helper.PathIsInside(s.uploadFolder, absolute) {
		s.logger.Error("the command would be stored outside the upload folder", slog.String("path", absolute))
		return errors.NewAppError(http.StatusInternalServerError, "failed to store the command")
	}

	if err := helper.CreatePathIfNotExists(filepath.Dir(absolute)); err != nil {
		s.logger.Error("failed to create the command storage folder", slog.Any("error", err))
		return errors.NewAppError(http.StatusInternalServerError, "failed to store the command")
	}

	if err := os.WriteFile(absolute, command.Bytes, 0o600); err != nil {
		s.logger.Error("failed to write a binary command", slog.Any("error", err))
		return errors.NewAppError(http.StatusInternalServerError, "failed to store the command")
	}

	return nil
}

// readStoredFile reads the bytes of a binary command.
func (s *service) readStoredFile(command Command) ([]byte, error) {
	absolute := s.absoluteStoragePath(command.StoragePath)
	if !helper.PathIsInside(s.uploadFolder, absolute) {
		return nil, os.ErrNotExist
	}

	return os.ReadFile(absolute)
}

// removeCommandStorage removes every byte a command wrote.
func (s *service) removeCommandStorage(id uuid.UUID) {
	if err := os.RemoveAll(s.commandStorageDir(id)); err != nil {
		s.logger.Warn("failed to remove the command storage", slog.Any("error", err))
	}
}

// removeStoredFile removes the bytes of one command, and its folder when the
// folder is empty. The folder is the command's id, so it is shared by every
// name the command has had: it is not removed while it still holds a file.
func (s *service) removeStoredFile(command Command) {
	if command.StoragePath == "" {
		return
	}

	absolute := s.absoluteStoragePath(command.StoragePath)

	if err := os.Remove(absolute); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("failed to remove a command file from disk", slog.Any("error", err))
		return
	}

	// An empty folder is tidier than a leftover; a folder that still holds
	// the new version's file is not removed.
	if err := os.Remove(filepath.Dir(absolute)); err != nil && !os.IsNotExist(err) {
		s.logger.Debug("kept the command folder", slog.Any("error", err))
	}
}

// maxLibraryEntries bounds the library a single user can keep. It is not
// enforced in the schema (a count is not a constraint Postgres expresses
// cheaply), so a listing checks it and a create refuses beyond it.
func (s *service) libraryIsFull(userID uuid.UUID) bool {
	stored, ok := s.db.ListCommands(userID)
	return ok && len(stored) >= maxLibraryEntries
}
