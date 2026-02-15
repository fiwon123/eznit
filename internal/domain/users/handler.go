package users

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Get("/v1/users", h.getUsersHandler)
	r.Get("/v1/users/{id}", h.getUserHandler)
	r.Post("/v1/users", h.createUserHandler)
	r.Delete("/v1/users/{id}", h.deleteUserHandler)
	r.Put("/v1/users/{id}", h.updateUserHandler)
}

func (h *Handler) getUsersHandler(w http.ResponseWriter, r *http.Request) {
	users := h.service.GetUsers()

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "Application/json")
	json.NewEncoder(w).Encode(users)
}

func (h *Handler) getUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, found := h.service.GetUser(id)
	if !found {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var user CreateRequest

	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	resp, ok := h.service.CreateUser(user)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	resp, ok := h.service.DeleteUser(DeleteRequest{
		Id: id,
	})
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Id = id
	resp, ok := h.service.UpdateUser(req)
	if !ok {
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

}
