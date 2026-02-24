package users

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service *Service
	session *sessions.Service
	guard   *middleware.Guard
	logger  *logger.Config
}

func NewHandler(service *Service, session *sessions.Service, guard *middleware.Guard, logger *logger.Config) *Handler {
	return &Handler{
		service: service,
		session: session,
		guard:   guard,
		logger:  logger,
	}
}

func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthAdmin)

		r.Post("/v1/users", h.createUserHandler)
		r.Delete("/v1/users/{id}", h.deleteUserHandler)
		r.Put("/v1/users/{id}", h.updateUserHandler)
	})

	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthUser)

		r.Get("/v1/users", h.getUsersHandler)
		r.Get("/v1/users/{id}", h.getUserHandler)
	})

	r.Post("/v1/login", h.loginHandler)
	r.Post("/v1/signup", h.signupHandler)
}

func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("loginHandler")

	var request LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Error("failed to decode: ", slog.Any("error", err))
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	resp, ok := h.service.LoginUser(request)
	if !ok {
		h.logger.Error("login failed")
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.logger.Debug("user logged in!")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) signupHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("signupHandler")

	var request SignupRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Error("failed to decode: ", slog.Any("error", err))
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	resp, ok := h.service.SignupUser(request)
	if !ok {
		h.logger.Error("signup failed")
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	h.logger.Error("user signup!")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) getUsersHandler(w http.ResponseWriter, r *http.Request) {
	users := h.service.GetUsers()

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "Application/json")
	json.NewEncoder(w).Encode(users)
}

func (h *Handler) getUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	h.logger.Debug("getUserHandler ", slog.String("id", id))

	resp, found := h.service.GetUser(id)
	if !found {
		h.logger.Error("user not found")
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	h.logger.Debug("user found!")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) createUserHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("createUserHandler")

	var request CreateRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Error("failed to decode: ", slog.Any("error", err))
		http.Error(w, "Failed to decode request", http.StatusBadRequest)
		return
	}

	resp, ok := h.service.CreateUser(request)
	if !ok {
		h.logger.Error("create user failed")
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	h.logger.Debug("user created!")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	h.logger.Debug("deleteUserHandler", slog.String("id", id))

	resp, ok := h.service.DeleteUser(DeleteRequest{
		Id: id,
	})
	if !ok {
		h.logger.Error("delete user failed!")
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	h.logger.Debug("user deleted!")

	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	h.logger.Debug("updateUserHandler", slog.String("id", id))

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode: ", slog.Any("error", err))
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req.Id = id
	resp, ok := h.service.UpdateUser(req)
	if !ok {
		h.logger.Error("update user failed!")
		http.Error(w, resp.Msg, http.StatusInternalServerError)
		return
	}

	h.logger.Debug("user updated!")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)

}
