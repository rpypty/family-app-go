package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"family-app-go/internal/config"
	"family-app-go/pkg/logger"
)

type SupabaseAuth struct {
	baseURL  string
	apiKey   string
	client   *http.Client
	log      logger.Logger
	profiles ProfileSaver
	skipAuth bool
	mockUser User
}

type contextKey int

const (
	userIDKey contextKey = iota
	userKey
)

type userResponse struct {
	ID           string                 `json:"id"`
	Email        string                 `json:"email"`
	Sub          string                 `json:"sub"`
	UserMetadata map[string]interface{} `json:"user_metadata"`
	User         struct {
		ID  string `json:"id"`
		Sub string `json:"sub"`
	} `json:"user"`
}

type User struct {
	ID        string
	Email     string
	Name      string
	AvatarURL string
}

type ProfileSaver interface {
	UpsertProfile(ctx context.Context, userID, email, avatarURL string) error
}

func NewSupabaseAuth(cfg config.SupabaseConfig, profiles ProfileSaver, log logger.Logger) *SupabaseAuth {
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
		log:      log,
		profiles: profiles,
		skipAuth: cfg.SkipAuth,
		mockUser: User{
			ID:        strings.TrimSpace(cfg.MockUserID),
			Email:     strings.TrimSpace(cfg.MockUserEmail),
			Name:      strings.TrimSpace(cfg.MockUserName),
			AvatarURL: strings.TrimSpace(cfg.MockUserAvatar),
		},
	}
}

func (a *SupabaseAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMethod := r.Method
		requestPath := r.URL.Path

		if a.skipAuth {
			user := a.mockUser
			if user.ID == "" {
				a.log.Error("auth: mock auth user id not configured", "method", requestMethod, "path", requestPath)
				writeError(w, http.StatusInternalServerError, "auth_not_configured", "auth mock user id not configured")
				return
			}
			if a.profiles != nil {
				if err := a.profiles.UpsertProfile(r.Context(), user.ID, user.Email, user.AvatarURL); err != nil {
					a.log.Warn("auth: upsert profile failed", "user_id", user.ID, "err", err)
				}
			}
			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if a.baseURL == "" || a.apiKey == "" {
			a.log.Error(
				"auth: provider not configured",
				"method",
				requestMethod,
				"path",
				requestPath,
				"has_base_url",
				a.baseURL != "",
				"has_api_key",
				a.apiKey != "",
			)
			writeError(w, http.StatusInternalServerError, "auth_not_configured", "auth not configured")
			return
		}

		authorizationHeader := r.Header.Get("Authorization")
		token, ok := bearerToken(authorizationHeader)
		if !ok {
			a.log.Warn(
				"auth: missing or invalid bearer token",
				"method",
				requestMethod,
				"path",
				requestPath,
				"has_authorization_header",
				strings.TrimSpace(authorizationHeader) != "",
			)
			unauthorized(w)
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, a.baseURL+"/auth/v1/user", nil)
		if err != nil {
			a.log.Error("auth: build supabase auth request failed", "method", requestMethod, "path", requestPath, "err", err)
			unauthorized(w)
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("apikey", a.apiKey)

		resp, err := a.client.Do(req)
		if err != nil {
			a.log.Error("auth: request to supabase failed", "method", requestMethod, "path", requestPath, "err", err)
			unauthorized(w)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode >= http.StatusInternalServerError {
				a.log.Error("auth: supabase auth endpoint error", "method", requestMethod, "path", requestPath, "status_code", resp.StatusCode)
			} else {
				a.log.Warn("auth: supabase rejected token", "method", requestMethod, "path", requestPath, "status_code", resp.StatusCode)
			}
			unauthorized(w)
			return
		}

		var payload userResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			a.log.Error("auth: decode supabase auth response failed", "method", requestMethod, "path", requestPath, "err", err)
			unauthorized(w)
			return
		}

		userID := firstNonEmpty(payload.ID, payload.Sub, payload.User.ID, payload.User.Sub)
		if userID == "" {
			a.log.Warn("auth: supabase response missing user id", "method", requestMethod, "path", requestPath)
			unauthorized(w)
			return
		}

		user := User{
			ID:        userID,
			Email:     payload.Email,
			Name:      firstNonEmpty(stringFromMap(payload.UserMetadata, "name"), stringFromMap(payload.UserMetadata, "full_name")),
			AvatarURL: stringFromMap(payload.UserMetadata, "avatar_url"),
		}

		if a.profiles != nil {
			if err := a.profiles.UpsertProfile(r.Context(), user.ID, user.Email, user.AvatarURL); err != nil {
				a.log.Warn("auth: upsert profile failed", "user_id", user.ID, "err", err)
			}
		}

		ctx := WithUser(r.Context(), user)
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
	writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
}

func WithUser(ctx context.Context, user User) context.Context {
	ctx = context.WithValue(ctx, userKey, user)
	return context.WithValue(ctx, userIDKey, user.ID)
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserFromContext(ctx context.Context) (User, bool) {
	value := ctx.Value(userKey)
	user, ok := value.(User)
	if !ok || user.ID == "" {
		return User{}, false
	}
	return user, true
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

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	parsed, ok := value.(string)
	if !ok {
		return ""
	}
	return parsed
}
