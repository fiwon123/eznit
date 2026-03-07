package users

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// User Handler is responsible to handle incoming request after middleware
type Handler struct {
	service *Service
	session *sessions.Service
	guard   *middleware.Guard
	logger  *logger.Config
}

// Retunr a new User Handler
func NewHandler(service *Service, session *sessions.Service, guard *middleware.Guard, logger *logger.Config) *Handler {
	return &Handler{
		service: service,
		session: session,
		guard:   guard,
		logger:  logger,
	}
}

// All user routes
func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthAdmin)

		r.Delete("/v1/users/{id}", h.deleteUserHandler)
		r.Put("/v1/users/{id}", h.updateUserHandler)
	})

	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthUser)

		r.Get("/v1/users", h.getUsersHandler)
		r.Get("/v1/users/{id}", h.getUserHandler)
		r.Get("/v1/logout", h.logoutHandler)
	})

	r.Post("/v1/login", h.loginHandler)
	r.Post("/v1/signup", h.signupHandler)
}

func (h *Handler) loginHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("loginHandler")

	var request LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Warn("failed to decode: ", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusBadRequest, "Failed to decode request")
		return
	}

	resp, appError := h.service.LoginUser(r.Context(), request)
	if appError != nil {
		h.logger.Warn("login failed")
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("user logged in!")

	h.logger.Debug("login response", slog.Any("response", resp))
	helper.SendDataJson(w, http.StatusOK, resp)
}

func (h *Handler) logoutHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("logoutHandler")
	userID := r.Context().Value("user_id").(uuid.UUID)

	data := h.session.GetToken(r.Context(), userID)
	if data == nil {
		h.logger.Warn("failed to get token")
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid token")
		return
	}

	ok := h.session.UseToken(r.Context(), data.Token)

	if !ok {
		h.logger.Warn("logout failed")
		helper.SendErrorJson(w, http.StatusInternalServerError, "logout failed")
		return
	}

	h.logger.Debug("user logout!")

	helper.SendMessageJson(w, http.StatusOK, "user logout!")
}

func (h *Handler) signupHandler(w http.ResponseWriter, r *http.Request) {
	h.logger.Debug("signupHandler")

	var request SignupRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Warn("failed to decode: ", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusBadRequest, "Failed to decode request")
		return
	}

	message, appError := h.service.CreateUser(r.Context(), request)
	if appError != nil {
		h.logger.Warn("signup failed")
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Info("user signup!")

	helper.SendMessageJson(w, http.StatusOK, message)
}

func (h *Handler) getUsersHandler(w http.ResponseWriter, r *http.Request) {
	users, appError := h.service.GetUsers(r.Context())
	if appError != nil {
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	helper.SendDataJson(w, http.StatusOK, users)
}

func (h *Handler) getUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	h.logger.Debug("getUserHandler ", slog.String("id", id))

	parseID, err := uuid.Parse(id)
	if err != nil {
		h.logger.Error("get user failed!", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid id")
		return
	}

	resp, appError := h.service.GetUser(r.Context(), parseID)
	if appError != nil {
		h.logger.Warn("user not found")
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("user found!")

	helper.SendDataJson(w, http.StatusOK, resp)
}

func (h *Handler) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id := h.extractUserID(r)

	h.logger.Debug("deleteUserHandler", slog.String("id", id))

	parseID, err := uuid.Parse(id)
	if err != nil {
		h.logger.Error("delete user failed!", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid id")
		return
	}

	message, appError := h.service.DeleteUser(r.Context(), DeleteRequest{Id: parseID})
	if appError != nil {
		h.logger.Error("delete user failed!")
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("user deleted!")

	helper.SendMessageJson(w, http.StatusOK, message)
}

func (h *Handler) updateUserHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	h.logger.Debug("updateUserHandler", slog.String("id", id))

	var request UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Warn("failed to decode: ", slog.Any("error", err))
		helper.SendErrorJson(w, http.StatusBadRequest, "Failed to decode request")
		return
	}

	var err error
	request.Id, err = uuid.Parse(id)
	if err != nil {
		h.logger.Error("update user failed!", slog.String("error", err.Error()))
		helper.SendErrorJson(w, http.StatusBadRequest, "invalid id")
		return
	}

	message, appError := h.service.UpdateUser(r.Context(), request)
	if appError != nil {
		h.logger.Error("update user failed!")
		helper.SendErrorJson(w, appError.StatusCode(), appError.Error())
		return
	}

	h.logger.Debug("user updated!")

	helper.SendMessageJson(w, http.StatusOK, message)

}
