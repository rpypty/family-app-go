package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"family-app-go/internal/config"
)

type SupabaseAuth struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type contextKey int

const userIDKey contextKey = iota

type userResponse struct {
	ID   string `json:"id"`
	Sub  string `json:"sub"`
	User struct {
		ID  string `json:"id"`
		Sub string `json:"sub"`
	} `json:"user"`
}

func NewSupabaseAuth(cfg config.SupabaseConfig) *SupabaseAuth {
	baseURL := strings.TrimRight(cfg.URL, "/")
	timeout := cfg.AuthTimeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &SupabaseAuth{
		baseURL: baseURL,
		apiKey:  cfg.PublishableKey,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (a *SupabaseAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.baseURL == "" || a.apiKey == "" {
			http.Error(w, "auth not configured", http.StatusInternalServerError)
			return
		}

		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			unauthorized(w)
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, a.baseURL+"/auth/v1/user", nil)
		if err != nil {
			unauthorized(w)
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("apikey", a.apiKey)

		resp, err := a.client.Do(req)
		if err != nil {
			unauthorized(w)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			unauthorized(w)
			return
		}

		var payload userResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			unauthorized(w)
			return
		}

		userID := firstNonEmpty(payload.ID, payload.Sub, payload.User.ID, payload.User.Sub)
		if userID == "" {
			unauthorized(w)
			return
		}

		ctx := WithUserID(r.Context(), userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(value string) (string, bool) {
	parts := strings.Fields(value)
	if len(parts) != 2 {
		return "", false
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized"))
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	value := ctx.Value(userIDKey)
	userID, ok := value.(string)
	if !ok || userID == "" {
		return "", false
	}
	return userID, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
