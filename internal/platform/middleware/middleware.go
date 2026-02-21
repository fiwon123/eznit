package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/fiwon123/eznit/internal/domain/sessions"
)

type Guard struct {
	session *sessions.Service
}

func NewGuard(session *sessions.Service) *Guard {
	return &Guard{
		session: session,
	}
}

func (g *Guard) AuthUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			fmt.Println("Unauthorized: No token provided")
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")

		if !g.session.IsValid(token) {
			fmt.Println("Unauthorized: Invalid token")
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		userID, err := g.session.GetUserIDByToken(token)
		if err != nil {
			fmt.Println("Unauthorized")
			http.Error(w, "Unauthorized", 401)
			return
		}

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
