package middleware

import (
	"net/http"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/sessions"
)

type Guard struct {
	session *sessions.Service
}

func NewMiddleware(session *sessions.Service) *Guard {
	return &Guard{
		session: session,
	}
}

func (g *Guard) AuthUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		if !g.session.IsValid(token) {
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
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
