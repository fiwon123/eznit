package users

import (
	"encoding/json"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/internal/platform/middleware"
	"github.com/fiwon123/eznit/internal/platform/ratelimit"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// User Handler is responsible to handle incoming request after middleware
type Handler struct {
	service   *Service
	session   *sessions.Service
	guard     *middleware.Guard
	logger    *logger.Config
	rateLimit RateLimitPolicy
}

// RateLimitPolicy is the auth throttling the handler applies. A nil Limiter
// disables it (tests, or RATE_LIMIT_ENABLED=false).
type RateLimitPolicy struct {
	Limiter        *ratelimit.Limiter
	TrustProxy     bool
	LoginPerIP     int
	LoginPerEmail  int
	SignupPerIP    int
	SignupPerEmail int
}

// Retunr a new User Handler
func NewHandler(service *Service, session *sessions.Service, guard *middleware.Guard, logger *logger.Config, rateLimit RateLimitPolicy) *Handler {
	return &Handler{
		service:   service,
		session:   session,
		guard:     guard,
		logger:    logger,
		rateLimit: rateLimit,
	}
}

// All user routes
func (h *Handler) RegisterRoutes(r *chi.Mux) {
	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthAdmin)

		r.Get("/v1/users", h.getUsersHandler)
		r.Delete("/v1/users/{id}", h.deleteUserHandler)
		r.Put("/v1/users/{id}", h.updateUserHandler)
	})

	r.Group(func(r chi.Router) {
		r.Use(h.guard.AuthUser)

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

	if retry, limited := h.tooMany(r, "login", request.Email, h.rateLimit.LoginPerIP, h.rateLimit.LoginPerEmail); limited {
		h.logger.Warn("login rate limited")
		writeTooMany(w, retry)
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

	if retry, limited := h.tooMany(r, "signup", request.Email, h.rateLimit.SignupPerIP, h.rateLimit.SignupPerEmail); limited {
		h.logger.Warn("signup rate limited")
		writeTooMany(w, retry)
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

	callerID, ok := r.Context().Value("user_id").(uuid.UUID)
	if !ok {
		h.logger.Error("get user failed! no caller in the request context")
		helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	resp, appError := h.service.GetUser(r.Context(), parseID, callerID)
	if appError != nil {
		h.logger.Warn("get user failed", slog.Int("status", appError.StatusCode()))
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

	callerID, ok := r.Context().Value("user_id").(uuid.UUID)
	if !ok {
		h.logger.Error("delete user failed! no caller in the request context")
		helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	message, appError := h.service.DeleteUser(r.Context(), DeleteRequest{Id: parseID}, callerID)
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

// tooMany reports whether the request is over either the per-IP or the
// per-address limit. It counts the request on both keys and returns the longer
// of the two waits, so Retry-After is not optimistic. The address is the one
// that was submitted, whether or not an account exists — so throttling reveals
// nothing about an address.
func (h *Handler) tooMany(r *http.Request, action, email string, perIP, perEmail int) (time.Duration, bool) {
	if h.rateLimit.Limiter == nil {
		return 0, false
	}

	ip := clientIP(r, h.rateLimit.TrustProxy)
	var wait time.Duration
	limited := false

	for _, c := range []struct {
		key   string
		limit int
	}{
		{action + ":ip:" + ip, perIP},
		{action + ":email:" + normalizeEmail(email), perEmail},
	} {
		ok, retry := h.rateLimit.Limiter.Allow(c.key, c.limit)
		if !ok {
			limited = true
			if retry > wait {
				wait = retry
			}
		}
	}

	return wait, limited
}

// writeTooMany answers 429 with a Retry-After of whole seconds, at least one.
func writeTooMany(w http.ResponseWriter, retry time.Duration) {
	seconds := int(math.Ceil(retry.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	helper.SendErrorJson(w, http.StatusTooManyRequests, "too many attempts, try again later")
}

// clientIP is the caller's address. Behind a trusted proxy the forwarded header
// is used; otherwise RemoteAddr, which a direct client cannot forge.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
				return first
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// normalizeEmail collapses the address so " A@B " and "a@b" share a counter.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
