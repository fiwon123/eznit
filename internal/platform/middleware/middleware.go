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

// Guard is responsible to storage middleware data
// middleware is responsible to verify who is sending the request before procceed to handlers
type Guard struct {
	session *sessions.Service
	logger  *logger.Config
}

// Use to generate a new Guard Data
func NewGuard(session *sessions.Service, logger *logger.Config) *Guard {
	return &Guard{
		session: session,
		logger:  logger,
	}
}

// Check if user has a session to send request that must be logged
// And storage userID inside the request context after verified user exists as well
func (g *Guard) AuthUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		g.logger.Debug("Authorizing...")

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			g.logger.Warn("No token provided")
			helper.SendErrorJson(w, http.StatusUnauthorized, "No token provided")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		if !g.session.IsValid(token) {
			g.logger.Warn("Invalid token")
			helper.SendErrorJson(w, http.StatusUnauthorized, "Invalid token")
			return
		}

		userID, ok := g.session.GetUserIDByToken(token)
		if !ok {
			g.logger.Warn("Unauthorized")
			helper.SendErrorJson(w, http.StatusUnauthorized, "Unauthorized")
			return
		}

		g.logger.Debug("Authorized User", slog.String("userID", userID.String()))
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalUser is the guard a public read wears: the request goes through
// either way, and a session, when it carries one, is put in the context the
// same way AuthUser puts it there.
//
// It is what makes a public route widen for its caller instead of narrowing
// for everyone: the catalogue answers anonymous visitors, and a caller with a
// session also sees their own private rows.
//
// A token the API does not accept is treated as no token. Granting nothing is
// the safe failure here — the route is public, so the worst an unverified
// caller gets is the visitor view — and it keeps a stale token in a browser
// from turning the public catalogue into an error page. A write still wears
// AuthUser and answers 401.
func (g *Guard) OptionalUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		if !g.session.IsValid(token) {
			g.logger.Warn("Invalid token on a public route. Serving the visitor view")
			next.ServeHTTP(w, r)
			return
		}

		userID, ok := g.session.GetUserIDByToken(token)
		if !ok {
			g.logger.Warn("Unauthorized on a public route. Serving the visitor view")
			next.ServeHTTP(w, r)
			return
		}

		g.logger.Debug("Authorized User on a public route", slog.String("userID", userID.String()))
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Check if user is a admin and has permissions to make request
func (g *Guard) AuthAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			helper.SendErrorJson(w, http.StatusUnauthorized, "No token provided")
			return
		}

		// TODO
		helper.SendErrorJson(w, http.StatusUnauthorized, "Invalid token")

		next.ServeHTTP(w, r)
	})
}
