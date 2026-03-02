package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/sessions"
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
			g.logger.Error("Unauthorized: No token provided")
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		if !g.session.IsValid(token) {
			g.logger.Error("Unauthorized: Invalid token")
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		userID, ok := g.session.GetUserIDByToken(token)
		if !ok {
			g.logger.Error("Unauthorized")
			http.Error(w, "Unauthorized", 401)
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
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		// TODO
		http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)

		next.ServeHTTP(w, r)
	})
}
