package files

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/fiwon123/eznit/internal/platform/middleware"
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
	userID := r.Context().Value("user_id").(string)
	fmt.Println("user id: ", userID)

	resp, ok := h.service.GetFilesForUser(userID)
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) getFilesHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("getFilesHandler")

	resp, ok := h.service.GetFiles()
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) getFileHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, ok := h.service.GetFile(id)
	if !ok {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Prevents attackers from sending infinite data to crash your server.
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	// Parse the multipart form. 8MB stays in RAM, the rest goes to temp files.
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "File too big", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Could not find file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	userID := r.Context().Value("user_id").(string)

	contentType := header.Header.Get("Content-Type")
	resp, ok := h.service.StorageFile(file, header, contentType, userID)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, resp.Msg, header.Filename)
}

func (h *Handler) downloadHandler(w http.ResponseWriter, r *http.Request) {
	fileID := chi.URLParam(r, "id")
	userID := r.Context().Value("user_id").(string)

	fileData, ok := h.service.GetFileForUser(fileID, userID)
	if !ok {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	fmt.Println("open: ", fileData.Path)
	file, err := os.Open(fileData.Path)
	if err != nil {
		http.Error(w, "File not found", 404)
		return
	}
	defer file.Close()

	fullname := fileData.Name + "." + fileData.Ext
	w.Header().Set("Content-Disposition", "attachment; filename="+fullname)
	w.Header().Set("Content-Type", fileData.ContentType)

	io.Copy(w, file)
}

func (h *Handler) deleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	userID := r.Context().Value("user_id").(string)

	resp, ok := h.service.DeleteFileForUser(id, userID)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) updateHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "File too big", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Could not find file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	resp, ok := h.service.UpdateFile(id, file, header)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(resp)
}
