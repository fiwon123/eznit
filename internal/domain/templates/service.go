package templates

import (
	"context"
	"log/slog"
	"net/http"
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
}

// Return a new templates service
func NewService(db Repository, logger *logger.Config) *service {
	return &service{
		db:     db,
		logger: logger,
	}
}

// The limits of a template. A template is a folder structure of configuration
// files: small, text, and countable. The limits keep one publication from
// filling the table, and they are checked here so the rule is the same
// whatever the client sends.
const (
	maxFilesPerTemplate = 200
	maxFileBytes        = 256 * 1024
	maxTemplateBytes    = 4 * 1024 * 1024
	maxVariables        = 50
	maxNameLength       = 200
	maxDescription      = 1000
	maxPromptLength     = 500
	listLimit           = 100
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

	fileData := make([]TemplateFileData, 0, len(files))
	for _, file := range files {
		fileData = append(fileData, TemplateFileData{
			ID:           file.ID,
			Path:         file.Path,
			Content:      file.Content,
			IsExecutable: file.IsExecutable,
		})
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

	template, files, variables, appError := s.validate(request, uuid.Nil)
	if appError != nil {
		return nil, appError
	}

	template.UserID = userID
	template.Version = 1

	if !s.db.CreateTemplate(template, files, variables) {
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

	template, files, variables, appError := s.validate(request, id)
	if appError != nil {
		return nil, appError
	}

	template.UserID = userID
	template.Version = existing.Version + 1

	if !s.db.UpdateTemplate(template, files, variables) {
		return nil, errors.NewAppError(http.StatusInternalServerError, "failed to update template")
	}

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

	return "template deleted!", nil
}

// TemplateFiles returns the files of a template the caller may see, for the
// archive download.
func (s *service) TemplateFiles(ctx context.Context, id uuid.UUID) ([]TemplateFile, *errors.AppError) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		s.logger.Warn("no user in the request context")
		return nil, errors.NewAppError(http.StatusUnauthorized, "unauthorized")
	}

	s.logger.Debug("TemplateFiles", slog.String("id", id.String()))

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

// validate turns a publish request into the rows to store, or explains why
// it cannot.
//
// currentID is the template being updated, and uuid.Nil for a new one: the
// slug of a template is its own, so an update may keep it.
func (s *service) validate(request PublishRequest, currentID uuid.UUID) (Template, []TemplateFile, []TemplateVariable, *errors.AppError) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "template name is required")
	}

	if len(name) > maxNameLength {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "template name is too long")
	}

	description := strings.TrimSpace(request.Description)
	if len(description) > maxDescription {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "template description is too long")
	}

	kind := strings.TrimSpace(request.Kind)
	if kind == "" {
		kind = KindGeneric
	}

	if !IsKind(kind) {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "invalid template kind")
	}

	if len(request.Files) == 0 {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "a template needs at least one file")
	}

	if len(request.Files) > maxFilesPerTemplate {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "too many files in the template")
	}

	if len(request.Variables) > maxVariables {
		return Template{}, nil, nil, errors.NewAppError(http.StatusBadRequest, "too many variables in the template")
	}

	templateID := currentID
	if templateID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			s.logger.Error("failed to generate the template id", slog.Any("error", err))
			return Template{}, nil, nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
		}
		templateID = id
	}

	// The files and the variables belong to the template, so they carry its
	// id: the table has a foreign key on it, and a row without it is
	// refused by the database.
	files, appError := s.buildFiles(request.Files, templateID)
	if appError != nil {
		return Template{}, nil, nil, appError
	}

	variables, appError := s.buildVariables(request.Variables, templateID)
	if appError != nil {
		return Template{}, nil, nil, appError
	}

	slug, appError := s.resolveSlug(request.Slug, name, currentID, templateID)
	if appError != nil {
		return Template{}, nil, nil, appError
	}

	template := Template{
		ID:          templateID,
		Name:        name,
		Slug:        slug,
		Description: description,
		Kind:        kind,
		IsPublic:    request.IsPublic,
	}

	return template, files, variables, nil
}

// buildFiles turns the files of a request into rows, refusing the paths that
// cannot be written safely and the contents that are too big.
//
// Every row carries the id of the template it belongs to: the table has a
// foreign key on it, and the row without one is refused by the database.
func (s *service) buildFiles(requestFiles []PublishFile, templateID uuid.UUID) ([]TemplateFile, *errors.AppError) {
	files := make([]TemplateFile, 0, len(requestFiles))
	seen := make(map[string]bool, len(requestFiles))
	total := 0

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

		if len(requestFile.Content) > maxFileBytes {
			return nil, errors.NewAppError(http.StatusBadRequest, "file too big in the template")
		}
		total += len(requestFile.Content)

		if total > maxTemplateBytes {
			return nil, errors.NewAppError(http.StatusBadRequest, "template too big")
		}

		fileID, err := uuid.NewV7()
		if err != nil {
			s.logger.Error("failed to generate the file id", slog.Any("error", err))
			return nil, errors.NewAppError(http.StatusInternalServerError, "failed to publish template")
		}

		files = append(files, TemplateFile{
			ID:           fileID,
			TemplateID:   templateID,
			Path:         path,
			Content:      requestFile.Content,
			IsExecutable: requestFile.IsExecutable,
		})
	}

	return files, nil
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
