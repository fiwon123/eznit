package users

import "net/http"

func (h *Handler) extractUserID(r *http.Request) string {
	return r.PathValue("id")
}
