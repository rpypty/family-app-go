package common

import (
	"net/http"

	"family-app-go/internal/transport/httpserver/middleware"
)

type authMeResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func (h *Handlers) AuthMe(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	writeJSON(w, http.StatusOK, authMeResponse{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		AvatarURL: user.AvatarURL,
	})
}
