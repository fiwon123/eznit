package templates

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"path"

	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/errors"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler is responsible to handle all incoming request from the templates
// resource
type Handler struct {
	service *service
	guard   *middleware.Guard
	logger  *logger.Config
}

// Return a new templates Handler
func NewHandler(service *service, guard *middleware.Guard, logger *logger.Config) *Handler {
	return &Handler{
		service: service,
		guard:   guard,
		logger:  logger,
	}
}

// Templates routes
func (h *Handler) RegisterRoutes(r *chi.Mux) {

	r.Route("/v1/templates", func(r chi.Router) {

		// The reads are public: the catalogue is a page a visitor may open
		// without an account. The visibility rule is in the query — an
		// anonymous caller only ever matches `is_public = TRUE` — so opening
		// the route does not widen what it answers. The optional guard is
		// what puts a caller who does have a session back in the query.
		r.Group(func(r chi.Router) {
			r.Use(h.guard.OptionalUser)

			// The catalogue: every public template, plus the caller's own
			// when the request carries a session.
			r.Get("/", h.listTemplatesHandler)

			// A public template's detail, its archive and its files. A
			// private template that is not the caller's is a 404, anonymous
			// or not, so an id cannot be probed.
			r.Get("/{id}", h.getTemplateHandler)
			r.Get("/{id}/archive", h.downloadArchiveHandler)
			// One file of a template, for a binary file the detail does not
			// carry inline anywhere but in a publish-shaped response.
			r.Get("/{id}/files/{fileID}/content", h.downloadTemplateFileHandler)
		})

		// Everything that writes, and everything that answers with the
		// caller's own private templates, needs a session.
		r.Group(func(r chi.Router) {
			r.Use(h.guard.AuthUser)

			r.Post("/", h.publishTemplateHandler)

			// The caller's own templates, the private ones included. chi
			// prefers this static segment over the `{id}` route above, so
			// "mine" is never read as an id.
			r.Get("/mine", h.listMyTemplatesHandler)

			r.Put("/{id}", h.updateTemplateHandler)
			r.Delete("/{id}", h.deleteTemplateHandler)
		})
	})
}

func (h *Handler) listTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("listTemplatesHandler")

	kind := r.URL.Query().Get("kind")

	dataResp, appError := h.service.ListTemplates(r.Context(), kind)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) listMyTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("listMyTemplatesHandler")

	dataResp, appError := h.service.ListTemplatesForUser(r.Context())
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) getTemplateHandler(w http.ResponseWriter, r *http.Request) {
	id, appError := extractTemplateID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("getTemplateHandler ", slog.String("id", id.String()))

	dataResp, appError := h.service.GetTemplate(r.Context(), id)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) publishTemplateHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("publishTemplateHandler")

	// A template is a set of files and a set of questions: a body, not a
	// stream. The limit is the same one the files route uses, and it is the
	// boundary a flood of bytes has to meet.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	var request PublishRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Warn("failed to decode: ", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	dataResp, appError := h.service.PublishTemplate(r.Context(), request)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) updateTemplateHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("updateTemplateHandler")

	id, appError := extractTemplateID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	var request PublishRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Warn("failed to decode: ", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	dataResp, appError := h.service.UpdateTemplate(r.Context(), id, request)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) deleteTemplateHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("deleteTemplateHandler")

	id, appError := extractTemplateID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	message, appError := h.service.DeleteTemplate(r.Context(), id)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}

// downloadArchiveHandler answers with the whole template as a zip, built from
// the rows: the files are small text files, so the archive is assembled in
// memory and never touches the disk.
func (h *Handler) downloadArchiveHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("downloadArchiveHandler")

	id, appError := extractTemplateID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	files, appError := h.service.TemplateArchive(r.Context(), id)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)

	for _, file := range files {
		header := &zip.FileHeader{Name: file.Path, Method: zip.Deflate}
		if file.IsExecutable {
			header.SetMode(0755)
		}

		entry, err := archive.CreateHeader(header)
		if err != nil {
			h.logger.Error("failed to add a file to the archive", slog.Any("error", err))
			helper.SendErrorJson(w, http.StatusInternalServerError, "failed to build the archive")
			return
		}

		if _, err := entry.Write(file.Content); err != nil {
			h.logger.Error("failed to write a file to the archive", slog.Any("error", err))
			helper.SendErrorJson(w, http.StatusInternalServerError, "failed to build the archive")
			return
		}
	}

	if err := archive.Close(); err != nil {
		h.logger.Error("failed to close the archive", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusInternalServerError, "failed to build the archive")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=template.zip")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(buffer.Bytes()); err != nil {
		h.logger.Error("failed to send the archive", slog.Any("error", err))
	}
}

// downloadTemplateFileHandler sends one file of a template as it is stored:
// the column for a text file, the bytes on disk for a binary one.
func (h *Handler) downloadTemplateFileHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("downloadTemplateFileHandler")

	id, appError := extractTemplateID(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	fileID, err := uuid.Parse(r.PathValue("fileID"))
	if err != nil {
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid file id")
		return
	}

	file, content, appError := h.service.TemplateFileContent(r.Context(), id, fileID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	contentType := mime.TypeByExtension(path.Ext(file.Path))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(file.Path)}))
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(content); err != nil {
		h.logger.Error("failed to send a template file", slog.Any("error", err))
	}
}

// extractTemplateID reads the template id from the path and parses it.
func extractTemplateID(r *http.Request) (uuid.UUID, *errors.AppError) {
	id := r.PathValue("id")

	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, errors.NewAppError(http.StatusBadRequest, "invalid template id")
	}

	return parsed, nil
}
