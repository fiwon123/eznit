package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/sessions"
	"github.com/fiwon123/eznit/pkg/helper"
	"github.com/fiwon123/eznit/pkg/logger"
)

type Guard struct {
	session *sessions.Service
	logger  *logger.Config
}

func NewGuard(session *sessions.Service, logger *logger.Config) *Guard {
	return &Guard{
		session: session,
		logger:  logger,
	}
}

func (g *Guard) AuthUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		g.logger.Debug("Authorizing...")

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			g.logger.Warn("Unauthorized: No token provided")
			helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized: No token provided")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		if !g.session.IsValid(token) {
			g.logger.Warn("Unauthorized: Invalid token")
			helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized: Invalid token")
			return
		}

		userID, ok := g.session.GetUserIDByToken(token)
		if !ok {
			g.logger.Warn("Unauthorized")
			helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		g.logger.Debug("Authorized User: ", slog.String("userID", userID))
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (g *Guard) AuthAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized: No token provided")
			return
		}

		// TODO
		helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized: Invalid token")

		next.ServeHTTP(w, r)
	})
}
