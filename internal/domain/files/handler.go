package files

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service
	guard   *middleware.Guard
}

func NewHandler(service *service, guard *middleware.Guard) *Handler {
	return &Handler{
		service: service,
		guard:   guard,
	}
}

func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthUser)

		r.Route("/v1/files", func(r chi.Router) {
			r.Post("/", h.uploadHandler)
			r.Get("/", h.getFilesHandler)

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

func (h *Handler) getFilesHandler(w http.ResponseWriter, r *http.Request) {
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

	file, err := os.Open(fileData.Path)
	if err != nil {
		http.Error(w, "File not found", 404)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+fileData.Name)
	w.Header().Set("Content-Type", fileData.ContentType)

	io.Copy(w, file)
}

func (h *Handler) deleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, ok := h.service.DeleteFile(id)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
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
