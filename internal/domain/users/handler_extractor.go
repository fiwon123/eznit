package users

import "net/http"

// extract UserID by request
func (h *Handler) extractUserID(r *http.Request) string {
	return r.PathValue("id")
}
