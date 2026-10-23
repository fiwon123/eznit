package files

import (
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Files handler reponsible to handle all incoming request from files handler
type Handler struct {
	service *service
	guard   *middleware.Guard
	logger  *logger.Config
}

// Return a new Files Handler
func NewHandler(service *service, guard *middleware.Guard, logger *logger.Config) *Handler {
	return &Handler{
		service: service,
		guard:   guard,
		logger:  logger,
	}
}

// Files routes
func (h *Handler) RegisterRoutes(r *chi.Mux) {

	r.Route("/v1/files", func(r chi.Router) {

		r.Group(func(r chi.Router) {
			r.Use(h.guard.AuthUser)

			// The list answers the caller's own files, and it needs a
			// session like every other route on this resource.
			r.Get("/", h.getFilesHandler)
			r.Post("/", h.uploadHandler)

			// An alias of the list above, kept for the clients that name
			// the caller's files explicitly: the web UI and
			// `eznit list --me`.
			r.Get("/me", h.getFilesHandler)
		})

		r.Route("/{id}", func(r chi.Router) {

			r.Group(func(r chi.Router) {
				r.Use(h.guard.AuthUser)

				// The download is a read of somebody's file: it needs the
				// same session as every other route on this resource, and
				// the service resolves it for the caller, so a token for
				// another user's file does not download it.
				r.Get("/content", h.downloadHandler)
				r.Get("/", h.getFileHandler)
				r.Put("/", h.updateHandler)
				r.Delete("/", h.deleteHandler)
			})
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthAdmin)
	})
}

// getFilesHandler lists the caller's own files. Both list routes land
// here: `/` and its `/me` alias.
func (h *Handler) getFilesHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("getFilesHandler")

	dataResp, appError := h.service.GetFilesForUser(r.Context())
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) getFileHandler(w http.ResponseWriter, r *http.Request) {
	id := extractFileID(r)
	h.logger.Debug("getFileHandler ", slog.String("id", id))

	parseID, err := uuid.Parse(id)
	if err != nil {
		h.logger.Warn("failed to parse file id", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid file id")
		return
	}

	dataResp, appError := h.service.GetFile(r.Context(), parseID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusFound, dataResp)
}

func (h *Handler) uploadHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("uploadHandler")

	// Prevents attackers from sending infinite data to crash your server.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	file, header, contentType, appError := extractFile(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}
	defer file.Close()

	message, appError := h.service.StorageFile(r.Context(), file, header, contentType)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}

func (h *Handler) downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileID := extractFileID(r)
	h.logger.Debug("downloadHandler ", slog.String("id", fileID))

	parseID, err := uuid.Parse(fileID)
	if err != nil {
		h.logger.Warn("failed to parse file id", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid file id")
		return
	}

	fileData, appError := h.service.GetFileForUser(r.Context(), parseID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("open: ", slog.String("path", fileData.Path))
	file, err := os.Open(fileData.Path)
	if err != nil {
		h.logger.Debug("error to open file", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()

	h.logger.Debug("found file", slog.Any("fileData", fileData))

	var fullname string
	if fileData.Ext != "" {
		fullname = fileData.Name + "." + fileData.Ext
	} else {
		fullname = fileData.Name
	}

	w.Header().Set("Content-Disposition", "attachment; filename="+fullname)
	w.Header().Set("Content-Type", fileData.ContentType)
	w.WriteHeader(http.StatusOK)

	io.Copy(w, file)
}

func (h *Handler) deleteHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(uuid.UUID)
	id := extractFileID(r)
	h.logger.Debug("deleteHandler ", slog.String("id", id))

	parseID, err := uuid.Parse(id)
	if err != nil {
		h.logger.Warn("failed to parse file id", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid file id")
		return
	}

	message, appError := h.service.DeleteFileForUser(r.Context(), parseID, userID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}

func (h *Handler) updateHandler(w http.ResponseWriter, r *http.Request) {

	id := extractFileID(r)
	h.logger.Debug("updateHandler ", slog.String("id", id))

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	file, header, _, appError := extractFile(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}
	defer file.Close()

	parseID, err := uuid.Parse(id)
	if err != nil {
		h.logger.Warn("failed to parse file id", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid file id")
		return
	}

	userID := r.Context().Value("user_id").(uuid.UUID)

	message, appError := h.service.UpdateFile(r.Context(), file, header, parseID, userID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}
