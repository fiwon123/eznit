package files

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *service
}

func NewHandler(service *service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Get("/v1/files", h.getFilesHandler)
	r.Post("/v1/files", h.uploadHandler)
	r.Delete("/v1/files/{id}", h.deleteHandler)
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

	resp, ok := h.service.StorageFile(file, header)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, resp.Msg, header.Filename)
}

func (h *Handler) deleteHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, ok := h.service.DeleteFile(id)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
	}

	json.NewEncoder(w).Encode(resp)
}
