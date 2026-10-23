package templates

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/google/uuid"
)

// service is responsible for the rules of the templates resource: what a
// valid template is, who may see it, and who may change it.
type service struct {
	db     Repository
	logger *logger.Config
	// uploadFolder is the root the binary files of a template are written
	// under, the same root the files domain stores uploads in. A text-only
	// template never writes to it.
	uploadFolder string
}

// Return a new templates service
func NewService(db Repository, uploadFolder string, logger *logger.Config) *service {
	return &service{
		db:           db,
		uploadFolder: uploadFolder,
		logger:       logger,
	}
}

// The limits of a template. A template is a folder structure of configuration
// files: small, text, and countable. The limits keep one publication from
// filling the table, and they are checked here so the rule is the same
// whatever the client sends.
const (
	maxFilesPerTemplate = 200
	// A text file is configuration and small; a binary file is an image or a
	// compiled helper, and is allowed to be bigger. The template as a whole is
	// capped once, counting both.
	maxTextFileBytes       = 256 * 1024
	maxBinaryFileBytes     = 10 * 1024 * 1024
	maxTextTemplateBytes   = 4 * 1024 * 1024
	maxBinaryTemplateBytes = 32 * 1024 * 1024
	maxVariables           = 50
	maxCommandsPerTemplate = 50
	maxNameLength          = 200
	maxDescription         = 1000
	maxPromptLength        = 500
	listLimit              = 100
)

// slugPattern is the shape of a slug: lowercase letters, digits and dashes.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// variableKeyPattern is the shape of a variable key: an upper case name, so
// the placeholder in a file reads as one: {{PROJECT_NAME}}.
var variableKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// userIDFromContext returns the user the guard put in the context. The guard
// is the only thing that sets it, so a missing value means the request never
// went through the guard — the route is registered without it. The caller is
// rejected instead of the request panicking on the type assertion, so a
// misregistered route fails closed.
func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value("user_id").(uuid.UUID)
	return userID, ok
}

// ListTemplates returns the catalogue: the public templates and the caller's
// own. An empty kind means every kind.
func (s *service) ListTemplates(ctx context.Context, kind string) (ListResponse, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return ListResponse{}, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("ListTemplates", slog.String("kind", kind))

	if kind != "" && !IsKind(kind) {
		return ListResponse{}, errors.NewAppError(http.StatusBadRequest, "invalid template kind")
	}

	templates, ok := s.db.ListTemplates(userID, kind, listLimit)
	if !ok {
		return ListResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to get templates")
	}

	return listResponse(templates), nil
}

// ListTemplatesForUser returns only the caller's own templates, the private
// ones included.
func (s *service) ListTemplatesForUser(ctx context.Context) (ListResponse, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return ListResponse{}, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("ListTemplatesForUser", slog.String("userID", userID.String()))

	templates, ok := s.db.ListTemplatesForUser(userID)
	if !ok {
		return ListResponse{}, errors.NewAppError(http.StatusInternalServerError, "failed to get templates")
	}

	return listResponse(templates), nil
}

// listResponse turns the templates into the catalogue the client sees. The
// file count is not read from a column: it is the number of files the
// repository holds for each template, read with the templates themselves, so
// the catalogue and the detail page cannot disagree.
func listResponse(templates []Template) ListResponse {
	resp := make([]TemplateData, 0, len(templates))

	for _, template := range templates {
		resp = append(resp, TemplateData{
			ID:          template.ID,
			Name:        template.Name,
			Slug:        template.Slug,
			Description: template.Description,
			Kind:        template.Kind,
			Version:     template.Version,
			IsPublic:    template.IsPublic,
			Owner:       template.UserID.String(),
			OwnerEmail:  template.OwnerEmail,
			FileCount:   template.FileCount,
			CreatedAt:   template.CreatedAt,
			UpdatedAt:   template.UpdatedAt,
		})
	}

	return ListResponse{
		Data:  resp,
		Total: len(resp),
	}
}

// GetTemplate returns a template with its files and its variables.
//
// A template the caller may not see is a template that does not exist as far
// as the caller is concerned: the same answer for "no such template" and for
// "somebody else's private template", so an id cannot be probed.
func (s *service) GetTemplate(ctx context.Context, id uuid.UUID) (*TemplateDetail, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("GetTemplate", slog.String("id", id.String()))

	template, ok := s.db.GetTemplate(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	if !template.IsPublic && template.UserID != userID {
		s.logger.Warn("template not visible for this user")
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	return s.detail(template)
}

// detail reads the files and the variables of a template and assembles it.
func (s *service) detail(template *Template) (*TemplateDetail, *errors.AppError) {
	files, ok := s.db.GetTemplateFiles(template.ID)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template files")
	}

	variables, ok := s.db.GetTemplateVariables(template.ID)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template variables")
	}

	commands, ok := s.db.GetTemplateCommands(template.ID)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template commands")
	}

	fileData := make([]TemplateFileData, 0, len(files))
	for _, file := range files {
		data := TemplateFileData{
			ID:           file.ID,
			Path:         file.Path,
			Kind:         file.Kind,
			Size:         file.Size,
			Content:      file.Content,
			IsExecutable: file.IsExecutable,
		}

		if file.Kind == FileKindBinary {
			bytes, err := s.readStoredFile(file)
			if err != nil {
				s.logger.Error("failed to read a binary template file", slog.Any("error", err))
				return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template files")
			}
			data.ContentBase64 = base64.StdEncoding.EncodeToString(bytes)
		}

		fileData = append(fileData, data)
	}

	commandData := make([]TemplateCommandData, 0, len(commands))
	for _, command := range commands {
		data := TemplateCommandData{
			CommandID:    command.CommandID,
			TargetPath:   command.TargetPath,
			Name:         command.Name,
			Kind:         command.Kind,
			Size:         command.Size,
			Content:      command.Content,
			IsExecutable: command.IsExecutable,
		}

		if command.Kind == FileKindBinary {
			bytes, err := s.contentOfCommand(command)
			if err != nil {
				s.logger.Error("failed to read a binary template command", slog.Any("error", err))
				return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template commands")
			}
			data.ContentBase64 = base64.StdEncoding.EncodeToString(bytes)
		}

		commandData = append(commandData, data)
	}

	variableData := make([]TemplateVariableData, 0, len(variables))
	for _, variable := range variables {
		variableData = append(variableData, TemplateVariableData{
			ID:           variable.ID,
			Key:          variable.Key,
			Prompt:       variable.Prompt,
			DefaultValue: variable.DefaultValue,
			Required:     variable.Required,
			Position:     variable.Position,
		})
	}

	return &TemplateDetail{
		TemplateData: TemplateData{
			ID:          template.ID,
			Name:        template.Name,
			Slug:        template.Slug,
			Description: template.Description,
			Kind:        template.Kind,
			Version:     template.Version,
			IsPublic:    template.IsPublic,
			Owner:       template.UserID.String(),
			OwnerEmail:  template.OwnerEmail,
			FileCount:   len(fileData),
			CreatedAt:   template.CreatedAt,
			UpdatedAt:   template.UpdatedAt,
		},
		Files:     fileData,
		Variables: variableData,
		Commands:  commandData,
	}, nil
}

// PublishTemplate stores a new template for the caller.
func (s *service) PublishTemplate(ctx context.Context, request PublishRequest) (*TemplateDetail, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("PublishTemplate", slog.String("userID", userID.String()))

	template, files, variables, commands, appError := s.validate(request, userID, uuid.Nil)
	if appError != nil {
		return nil, appError
	}

	template.UserID = userID
	template.Version = 1

	if appError := s.writeBinaryFiles(files); appError != nil {
		return nil, appError
	}

	if !s.db.CreateTemplate(template, files, variables, commands) {
		// The rows were not written, so nothing points at the bytes.
		s.removeTemplateStorage(template.ID)
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
	}

	stored, ok := s.db.GetTemplate(template.ID)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
	}

	return s.detail(stored)
}

// UpdateTemplate replaces a template the caller owns. The version goes up by
// one, so a client can tell two versions of the same template apart.
func (s *service) UpdateTemplate(ctx context.Context, id uuid.UUID, request PublishRequest) (*TemplateDetail, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("UpdateTemplate", slog.String("id", id.String()))

	existing, ok := s.db.GetTemplate(id)
	if !ok || existing.UserID != userID {
		// A template the caller does not own is a template that does not
		// exist as far as the caller is concerned.
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	template, files, variables, commands, appError := s.validate(request, userID, id)
	if appError != nil {
		return nil, appError
	}

	template.UserID = userID
	template.Version = existing.Version + 1

	// The rows of the version being replaced, so their bytes can go once the
	// new version is stored.
	replaced, ok := s.db.GetTemplateFiles(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to update template")
	}

	if appError := s.writeBinaryFiles(files); appError != nil {
		return nil, appError
	}

	if !s.db.UpdateTemplate(template, files, variables, commands) {
		// The new rows were not written: the bytes they would have pointed
		// at are unreferenced.
		s.removeStoredFiles(files)
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to update template")
	}

	// The old rows are gone; their bytes are not.
	s.removeStoredFiles(replaced)

	stored, ok := s.db.GetTemplate(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to update template")
	}

	return s.detail(stored)
}

// DeleteTemplate removes a template the caller owns.
func (s *service) DeleteTemplate(ctx context.Context, id uuid.UUID) (string, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return "", errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("DeleteTemplate", slog.String("id", id.String()))

	if !s.db.DeleteTemplate(id, userID) {
		return "", errors.NewAppError(http.StatusNotFound, "template not found")
	}

	// The files and the variables went with the row (the schema cascades);
	// the binary bytes are on disk and have to go too.
	s.removeTemplateStorage(id)

	return "template deleted!", nil
}

// visibleFiles returns the files of a template the caller may see.
//
// A private template that is not the caller's is reported as not found: the
// fact that it exists is not theirs to learn.
func (s *service) visibleFiles(ctx context.Context, id uuid.UUID) ([]TemplateFile, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("visibleFiles", slog.String("id", id.String()))

	template, ok := s.db.GetTemplate(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	if !template.IsPublic && template.UserID != userID {
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	files, ok := s.db.GetTemplateFiles(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template files")
	}

	return files, nil
}

// TemplateFiles returns the files of a template the caller may see, for the
// archive download.
func (s *service) TemplateFiles(ctx context.Context, id uuid.UUID) ([]TemplateFile, *errors.AppError) {
	return s.visibleFiles(ctx, id)
}

// TemplateFileContent returns one file of a template and its bytes: the column
// for a text file, the disk for a binary one.
func (s *service) TemplateFileContent(ctx context.Context, templateID uuid.UUID, fileID uuid.UUID) (*TemplateFile, []byte, *errors.AppError) {
	files, appError := s.visibleFiles(ctx, templateID)
	if appError != nil {
		return nil, nil, appError
	}

	for _, file := range files {
		if file.ID != fileID {
			continue
		}

		bytes, err := s.contentOf(file)
		if err != nil {
			s.logger.Error("failed to read a template file", slog.Any("error", err))
			return nil, nil, errors.NewAppError(http.StatusInternalServerError, "failed to read the file")
		}

		return &file, bytes, nil
	}

	return nil, nil, errors.NewAppError(http.StatusNotFound, "file not found")
}

// ArchiveEntry is one file of a template with its bytes, for the archive.
type ArchiveEntry struct {
	Path         string
	IsExecutable bool
	Content      []byte
}

// TemplateArchive returns every file of a template with its bytes, read from
// the column or from the disk.
func (s *service) TemplateArchive(ctx context.Context, id uuid.UUID) ([]ArchiveEntry, *errors.AppError) {
	files, appError := s.visibleFiles(ctx, id)
	if appError != nil {
		return nil, appError
	}

	entries := make([]ArchiveEntry, 0, len(files))
	for _, file := range files {
		bytes, err := s.contentOf(file)
		if err != nil {
			s.logger.Error("failed to read a template file for the archive", slog.Any("error", err))
			return nil, errors.NewAppError(http.StatusInternalServerError, "failed to build the archive")
		}

		entries = append(entries, ArchiveEntry{
			Path:         file.Path,
			IsExecutable: file.IsExecutable,
			Content:      bytes,
		})
	}

	commands, appError := s.visibleCommands(ctx, id)
	if appError != nil {
		return nil, appError
	}

	for _, command := range commands {
		bytes, err := s.contentOfCommand(command)
		if err != nil {
			s.logger.Error("failed to read a template command for the archive", slog.Any("error", err))
			return nil, errors.NewAppError(http.StatusInternalServerError, "failed to build the archive")
		}

		entries = append(entries, ArchiveEntry{
			Path:         command.TargetPath,
			IsExecutable: command.IsExecutable,
			Content:      bytes,
		})
	}

	return entries, nil
}

// templateStorageDir is where every binary file of a template lives.
func (s *service) templateStorageDir(templateID uuid.UUID) string {
	return filepath.Join(s.uploadFolder, "templates", templateID.String())
}

// storagePathOf is where a binary file lives, relative to the upload root.
//
// The separator is a forward slash, always: the value travels in a database
// row and is read on machines that are not this one, and a backslash is not a
// path separator there. The file's own directory is its id, so two files with
// the same name in different folders cannot collide.
func storagePathOf(templateID, fileID uuid.UUID, filePath string) string {
	return path.Join("templates", templateID.String(), fileID.String(), path.Base(filePath))
}

// absoluteStoragePath is the location a row points at, on this machine.
func (s *service) absoluteStoragePath(storagePath string) string {
	return filepath.Join(s.uploadFolder, filepath.FromSlash(storagePath))
}

// writeBinaryFiles writes the bytes of the binary files of a template.
//
// It runs before the database is touched; the caller removes what it wrote if
// the database write fails, so a failed publish leaves no unreferenced bytes
// behind.
func (s *service) writeBinaryFiles(files []TemplateFile) *errors.AppError {
	for _, file := range files {
		if file.Kind != FileKindBinary {
			continue
		}

		absolute := s.absoluteStoragePath(file.StoragePath)
		if !helper.PathIsInside(s.uploadFolder, absolute) {
			s.logger.Error("the file would be stored outside the upload folder", slog.String("path", absolute))
			s.removeStoredFiles(files)
			return errors.NewAppError(http.StatusInternalServerError, "failed to store the template")
		}

		if err := helper.CreatePathIfNotExists(filepath.Dir(absolute)); err != nil {
			s.logger.Error("failed to create the template storage folder", slog.Any("error", err))
			s.removeStoredFiles(files)
			return errors.NewAppError(http.StatusInternalServerError, "failed to store the template")
		}

		if err := os.WriteFile(absolute, file.Bytes, 0o600); err != nil {
			s.logger.Error("failed to write a binary template file", slog.Any("error", err))
			s.removeStoredFiles(files)
			return errors.NewAppError(http.StatusInternalServerError, "failed to store the template")
		}
	}

	return nil
}

// removeTemplateStorage removes every byte a template wrote.
func (s *service) removeTemplateStorage(templateID uuid.UUID) {
	if err := os.RemoveAll(s.templateStorageDir(templateID)); err != nil {
		// The rows are already gone; an orphaned directory is a leak, not a
		// failure the caller can act on. It is logged and the operation
		// stands.
		s.logger.Warn("failed to remove the template storage", slog.Any("error", err))
	}
}

// removeStoredFiles removes the bytes of the given rows from disk.
func (s *service) removeStoredFiles(files []TemplateFile) {
	for _, file := range files {
		if file.StoragePath == "" {
			continue
		}

		directory := filepath.Dir(s.absoluteStoragePath(file.StoragePath))
		if err := os.RemoveAll(directory); err != nil {
			s.logger.Warn("failed to remove a template file from disk", slog.Any("error", err))
		}
	}
}

// readStoredFile reads the bytes of a binary file.
func (s *service) readStoredFile(file TemplateFile) ([]byte, error) {
	absolute := s.absoluteStoragePath(file.StoragePath)
	if !helper.PathIsInside(s.uploadFolder, absolute) {
		return nil, fmt.Errorf("the storage path %q is outside the upload folder", file.StoragePath)
	}

	return os.ReadFile(absolute)
}

// contentOf returns the bytes of a file: the column for a text file, the disk
// for a binary one.
func (s *service) contentOf(file TemplateFile) ([]byte, error) {
	if file.Kind != FileKindBinary {
		return []byte(file.Content), nil
	}

	return s.readStoredFile(file)
}

// contentOfCommand is contentOf for a command a template pulls in: a text
// command holds its content, a binary one lives under the upload root.
func (s *service) contentOfCommand(command TemplateCommand) ([]byte, error) {
	if command.Kind != FileKindBinary {
		return []byte(command.Content), nil
	}

	absolute := s.absoluteStoragePath(command.StoragePath)
	if !helper.PathIsInside(s.uploadFolder, absolute) {
		return nil, os.ErrNotExist
	}

	return os.ReadFile(absolute)
}

// visibleCommands returns the commands of a template the caller may see.
func (s *service) visibleCommands(ctx context.Context, id uuid.UUID) ([]TemplateCommand, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	template, ok := s.db.GetTemplate(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	if !template.IsPublic && template.UserID != userID {
		return nil, errors.NewAppError(http.StatusNotFound, "template not found")
	}

	commands, ok := s.db.GetTemplateCommands(id)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to get template commands")
	}

	return commands, nil
}

// buildCommands turns the command references of a request into rows.
//
// A reference is an id from the caller's own library and the path it takes in
// the project. The library is per user, so a reference to a command of
// somebody else is refused as unknown rather than forbidden: whether it
// exists is not the caller's to learn.
func (s *service) buildCommands(requestCommands []PublishCommand, userID uuid.UUID, templateID uuid.UUID) ([]TemplateCommand, *errors.AppError) {
	if len(requestCommands) > maxCommandsPerTemplate {
		return nil, errors.NewAppError(http.StatusBadRequest, "too many commands in the template")
	}

	commands := make([]TemplateCommand, 0, len(requestCommands))
	seenPaths := make(map[string]bool, len(requestCommands))
	seenIDs := make(map[uuid.UUID]bool, len(requestCommands))
	ids := make([]uuid.UUID, 0, len(requestCommands))

	for _, requestCommand := range requestCommands {
		targetPath, ok := helper.SafeRelPath(strings.TrimSpace(requestCommand.TargetPath))
		if !ok {
			return nil, errors.NewAppError(http.StatusBadRequest, "a command path in the template cannot be written safely")
		}

		if seenPaths[targetPath] {
			return nil, errors.NewAppError(http.StatusBadRequest, "duplicate command path in the template: "+targetPath)
		}
		seenPaths[targetPath] = true

		if requestCommand.CommandID == uuid.Nil {
			return nil, errors.NewAppError(http.StatusBadRequest, "a command reference needs a command id")
		}

		if seenIDs[requestCommand.CommandID] {
			return nil, errors.NewAppError(http.StatusBadRequest, "a command is referenced twice in the template")
		}
		seenIDs[requestCommand.CommandID] = true
		ids = append(ids, requestCommand.CommandID)

		commands = append(commands, TemplateCommand{
			TemplateID: templateID,
			CommandID:  requestCommand.CommandID,
			TargetPath: targetPath,
		})
	}

	if len(ids) == 0 {
		return commands, nil
	}

	owned, ok := s.db.OwnedCommandIDs(userID, ids)
	if !ok {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to validate the template commands")
	}

	for _, id := range ids {
		if !owned[id] {
			return nil, errors.NewAppError(http.StatusBadRequest, "unknown command in the template")
		}
	}

	return commands, nil
}

// validate turns a publish request into the rows to store, or explains why
// it cannot.
//
// currentID is the template being updated, and uuid.Nil for a new one: the
// slug of a template is its own, so an update may keep it.
func (s *service) validate(request PublishRequest, userID uuid.UUID, currentID uuid.UUID) (Template, []TemplateFile, []TemplateVariable, []TemplateCommand, *errors.AppError) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "template name is required")
	}

	if len(name) > maxNameLength {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "template name is too long")
	}

	description := strings.TrimSpace(request.Description)
	if len(description) > maxDescription {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "template description is too long")
	}

	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		kind = KindGeneric
	}

	if !IsKind(kind) {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "invalid template kind")
	}

	if len(request.Files) == 0 {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "a template needs at least one file")
	}

	if len(request.Files) > maxFilesPerTemplate {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "too many files in the template")
	}

	if len(request.Variables) > maxVariables {
		return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "too many variables in the template")
	}

	templateID := currentID
	if templateID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			s.logger.Error("failed to generate the template id", slog.Any("error", err))
			return Template{}, nil, nil, nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
		}
		templateID = id
	}

	// The files and the variables belong to the template, so they carry its
	// id: the table has a foreign key on it, and a row without it is
	// refused by the database.
	files, appError := s.buildFiles(request.Files, templateID)
	if appError != nil {
		return Template{}, nil, nil, nil, appError
	}

	variables, appError := s.buildVariables(request.Variables, templateID)
	if appError != nil {
		return Template{}, nil, nil, nil, appError
	}

	commands, appError := s.buildCommands(request.Commands, userID, templateID)
	if appError != nil {
		return Template{}, nil, nil, nil, appError
	}

	// A template cannot put two things at one path: a command target that
	// a file already takes, or two commands aimed at the same path, would
	// write one over the other at apply time.
	filePaths := make(map[string]bool, len(files))
	for _, file := range files {
		filePaths[file.Path] = true
	}

	for _, command := range commands {
		if filePaths[command.TargetPath] {
			return Template{}, nil, nil, nil, errors.NewAppError(http.StatusBadRequest, "a command target path is also a file path: "+command.TargetPath)
		}
	}

	slug, appError := s.resolveSlug(request.Slug, name, currentID, templateID)
	if appError != nil {
		return Template{}, nil, nil, nil, appError
	}

	template := Template{
		ID:          templateID,
		Name:        name,
		Slug:        slug,
		Description: description,
		Kind:        kind,
		IsPublic:    request.IsPublic,
	}

	return template, files, variables, commands, nil
}

// buildFiles turns the files of a request into rows, refusing the paths that
// cannot be written safely and the contents that are too big.
//
// A text file carries its content inline. A binary file carries it in
// content_base64 and its bytes are written to disk by writeBinaryFiles: the
// row holds the location, not the bytes.
//
// Every row carries the id of the template it belongs to: the table has a
// foreign key on it, and the row without one is refused by the database.
func (s *service) buildFiles(requestFiles []PublishFile, templateID uuid.UUID) ([]TemplateFile, *errors.AppError) {
	files := make([]TemplateFile, 0, len(requestFiles))
	seen := make(map[string]bool, len(requestFiles))
	textTotal, total := 0, 0

	for _, requestFile := range requestFiles {
		path, ok := helper.SafeRelPath(strings.TrimSpace(requestFile.Path))
		if !ok {
			s.logger.Warn("refused a template file path", slog.String("path", requestFile.Path))
			return nil, errors.NewAppError(http.StatusBadRequest, "invalid file path in the template")
		}

		if seen[path] {
			return nil, errors.NewAppError(http.StatusBadRequest, "duplicate file path in the template")
		}
		seen[path] = true

		kind := strings.TrimSpace(requestFile.Kind)
		if kind == "" {
			kind = FileKindText
		}

		if !IsFileKind(kind) {
			return nil, errors.NewAppError(http.StatusBadRequest, "invalid file kind in the template")
		}

		fileID, err := uuid.NewV7()
		if err != nil {
			s.logger.Error("failed to generate the file id", slog.Any("error", err))
			return nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
		}

		file := TemplateFile{
			ID:           fileID,
			TemplateID:   templateID,
			Path:         path,
			Kind:         kind,
			IsExecutable: requestFile.IsExecutable,
		}

		if kind == FileKindBinary {
			if requestFile.Content != "" {
				return nil, errors.NewAppError(http.StatusBadRequest, "a binary file carries content_base64, not content")
			}

			bytes, appError := s.decodeBinaryFile(requestFile.ContentBase64)
			if appError != nil {
				return nil, appError
			}

			file.Size = int64(len(bytes))
			file.Bytes = bytes
			file.StoragePath = storagePathOf(templateID, fileID, path)
			total += len(bytes)

			if total > maxBinaryTemplateBytes {
				return nil, errors.NewAppError(http.StatusBadRequest, "template too big")
			}
		} else {
			if requestFile.ContentBase64 != "" {
				return nil, errors.NewAppError(http.StatusBadRequest, "a text file carries content, not content_base64")
			}

			if len(requestFile.Content) > maxTextFileBytes {
				return nil, errors.NewAppError(http.StatusBadRequest, "file too big in the template")
			}

			file.Content = requestFile.Content
			file.Size = int64(len(requestFile.Content))

			textTotal += len(requestFile.Content)
			total += len(requestFile.Content)

			if textTotal > maxTextTemplateBytes {
				return nil, errors.NewAppError(http.StatusBadRequest, "template too big")
			}

			if total > maxBinaryTemplateBytes {
				return nil, errors.NewAppError(http.StatusBadRequest, "template too big")
			}
		}

		files = append(files, file)
	}

	return files, nil
}

// decodeBinaryFile turns the base64 body of a binary file into bytes, and
// refuses one that is too big before decoding it.
func (s *service) decodeBinaryFile(encoded string) ([]byte, *errors.AppError) {
	if encoded == "" {
		return nil, errors.NewAppError(http.StatusBadRequest, "a binary file needs content_base64")
	}

	// base64 is four characters for every three bytes; checking the encoded
	// length first keeps a huge body from being decoded only to be refused.
	if int64(len(encoded)) > int64(maxBinaryFileBytes)*4/3+4 {
		return nil, errors.NewAppError(http.StatusBadRequest, "file too big in the template")
	}

	bytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.NewAppError(http.StatusBadRequest, "invalid base64 content in the template")
	}

	if len(bytes) > maxBinaryFileBytes {
		return nil, errors.NewAppError(http.StatusBadRequest, "file too big in the template")
	}

	return bytes, nil
}

// buildVariables turns the variables of a request into rows.
//
// Every row carries the id of the template it belongs to: the table has a
// foreign key on it, and the row without one is refused by the database.
func (s *service) buildVariables(requestVariables []PublishVariable, templateID uuid.UUID) ([]TemplateVariable, *errors.AppError) {
	variables := make([]TemplateVariable, 0, len(requestVariables))
	seen := make(map[string]bool, len(requestVariables))

	for position, requestVariable := range requestVariables {
		key := strings.TrimSpace(requestVariable.Key)
		if !variableKeyPattern.MatchString(key) {
			return nil, errors.NewAppError(http.StatusBadRequest, "invalid variable key")
		}

		if seen[key] {
			return nil, errors.NewAppError(http.StatusBadRequest, "duplicate variable key in the template")
		}
		seen[key] = true

		prompt := strings.TrimSpace(requestVariable.Prompt)
		if len(prompt) > maxPromptLength {
			return nil, errors.NewAppError(http.StatusBadRequest, "variable prompt is too long")
		}

		if prompt == "" {
			prompt = key
		}

		variableID, err := uuid.NewV7()
		if err != nil {
			s.logger.Error("failed to generate the variable id", slog.Any("error", err))
			return nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
		}

		variables = append(variables, TemplateVariable{
			ID:           variableID,
			TemplateID:   templateID,
			Key:          key,
			Prompt:       prompt,
			DefaultValue: requestVariable.DefaultValue,
			Required:     requestVariable.Required,
			Position:     position,
		})
	}

	return variables, nil
}

// resolveSlug finds the slug of a template: the one the client asked for, or
// one derived from the name. A slug that is taken gets a short suffix, so
// two users can publish a template with the same name.
//
// currentID is the template being updated, and uuid.Nil for a new one. It is
// not the same thing as templateID, the id the row will carry: a publish
// already knows the id its template will have, but that template does not
// exist yet, so asking the database about it is a question with no answer.
func (s *service) resolveSlug(requested string, name string, currentID uuid.UUID, templateID uuid.UUID) (string, *errors.AppError) {
	slug := strings.ToLower(strings.TrimSpace(requested))

	if slug == "" {
		// An update keeps the slug the template already has. The slug is
		// the address of the template, so renaming it should not move it.
		if currentID != uuid.Nil {
			if existing, ok := s.db.GetTemplate(currentID); ok {
				return existing.Slug, nil
			}
		}

		slug = slugify(name)
	}

	if !slugPattern.MatchString(slug) {
		return "", errors.NewAppError(http.StatusBadRequest, "invalid template slug")
	}

	// The template may keep its own slug: an update that does not change the
	// slug is not a conflict with itself.
	if s.db.SlugExists(slug) {
		owned := false

		if currentID != uuid.Nil {
			if existing, ok := s.db.GetTemplate(currentID); ok && existing.Slug == slug {
				owned = true
			}
		}

		if !owned {
			suffix := "-" + templateID.String()[:6]

			// The slug is a URL segment and the pattern caps it at 63
			// characters, so the suffix has to fit in what is left.
			if len(slug)+len(suffix) > 63 {
				slug = strings.Trim(slug[:63-len(suffix)], "-")
			}

			slug = slug + suffix
		}
	}

	return slug, nil
}

// slugify turns a name into a slug: lower case, words separated by dashes.
func slugify(name string) string {
	var b strings.Builder

	lastDash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}

	slug := strings.Trim(b.String(), "-")

	if slug == "" {
		slug = "template"
	}

	if len(slug) > 63 {
		slug = strings.Trim(slug[:63], "-")
	}

	return slug
}
