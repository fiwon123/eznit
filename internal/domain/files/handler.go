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
)

type Handler struct {
	service *service
	guard   *middleware.Guard
	logger  *logger.Config
}

func NewHandler(service *service, guard *middleware.Guard, logger *logger.Config) *Handler {
	return &Handler{
		service: service,
		guard:   guard,
		logger:  logger,
	}
}

func (h *Handler) RegisterRoutes(r *chi.Mux) {

	r.Route("/v1/files", func(r chi.Router) {
		r.Get("/", h.getFilesHandler)

		r.Group(func(r chi.Router) {
			r.Use(h.guard.AuthUser)

			r.Post("/", h.uploadHandler)
			r.Get("/me", h.getFilesForUserHandler)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getFileHandler)
				r.Put("/", h.updateHandler)
				r.Get("/content", h.downloadHandler)
				r.Delete("/", h.deleteHandler)
			})
		})
	})

	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthAdmin)
	})
}

func (h *Handler) getFilesForUserHandler(w http.ResponseWriter, r *http.Request) {
	userID := h.extractUserID(r)
	h.logger.Debug("user id: ", slog.String("id", userID))

	dataResp, appError := h.service.GetFilesForUser(userID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, dataResp)
}

func (h *Handler) getFilesHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("getFilesHandler")

	dataResp, appError := h.service.GetFiles()
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusFound, dataResp)
}

func (h *Handler) getFileHandler(w http.ResponseWriter, r *http.Request) {
	id := h.extractFileID(r)
	h.logger.Debug("getFileHandler ", slog.String("id", id))

	dataResp, appError := h.service.GetFile(id)
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

	file, header, contentType, appError := h.extractFile(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}
	defer file.Close()

	userID := h.extractUserID(r)
	message, appError := h.service.StorageFile(file, header, contentType, userID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}

func (h *Handler) downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileID := h.extractFileID(r)
	userID := h.extractUserID(r)
	h.logger.Debug("downloadHandler ", slog.String("id", fileID), slog.String("userID", userID))

	fileData, appError := h.service.GetFileForUser(fileID, userID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("open: ", slog.String("path", fileData.Path))
	file, err := os.Open(fileData.Path)
	if err != nil {
		helper.SendErrorJson(w, http.StatusNotFound, "file not found")
		return
	}
	defer file.Close()

	fullname := fileData.Name + "." + fileData.Ext
	w.Header().Set("Content-Disposition", "attachment; filename="+fullname)
	w.Header().Set("Content-Type", fileData.ContentType)

	io.Copy(w, file)
}

func (h *Handler) deleteHandler(w http.ResponseWriter, r *http.Request) {
	id := h.extractFileID(r)
	userID := h.extractUserID(r)
	h.logger.Debug("deleteHandler ", slog.String("id", id))

	message, appError := h.service.DeleteFileForUser(id, userID)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}

func (h *Handler) updateHandler(w http.ResponseWriter, r *http.Request) {

	id := h.extractFileID(r)
	h.logger.Debug("updateHandler ", slog.String("id", id))

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	file, header, _, appError := h.extractFile(r)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}
	defer file.Close()

	message, appError := h.service.UpdateFile(file, header, id)
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
	}

	helper.SendMessageJson(w, http.StatusOK, message)
}
