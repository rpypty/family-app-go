//go:build e2e
// +build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"family-app-go/internal/config"
	"family-app-go/internal/db"
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	todosdomain "family-app-go/internal/domain/todos"
	userdomain "family-app-go/internal/domain/user"
	analyticsrepo "family-app-go/internal/repository/analytics"
	expensesrepo "family-app-go/internal/repository/expenses"
	familyrepo "family-app-go/internal/repository/family"
	todosrepo "family-app-go/internal/repository/todos"
	userrepo "family-app-go/internal/repository/user"
	"family-app-go/internal/transport/httpserver"
	"family-app-go/internal/transport/httpserver/handler"
	"gorm.io/gorm"
)

type testEnv struct {
	server     *httptest.Server
	authServer *httptest.Server
	db         *gorm.DB
}

func setupE2E(t *testing.T) *testEnv {
	t.Helper()

	dsn := os.Getenv("E2E_DB_DSN")
	if dsn == "" {
		t.Skip("E2E_DB_DSN not set; skipping e2e tests")
	}

	authServer := newAuthServer(t)

	cfg := config.Config{
		DB: config.DBConfig{DSN: dsn},
		Supabase: config.SupabaseConfig{
			URL:            authServer.URL,
			PublishableKey: "test-key",
			AuthTimeout:    2 * time.Second,
		},
	}

	dbConn, err := db.NewPostgres(cfg.DB)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}

	if err := db.Migrate(dbConn); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := cleanDB(dbConn); err != nil {
		t.Fatalf("clean db: %v", err)
	}

	familyRepo := familyrepo.NewPostgres(dbConn)
	familyService := familydomain.NewService(familyRepo)
	expensesRepo := expensesrepo.NewPostgres(dbConn)
	expensesService := expensesdomain.NewService(expensesRepo)
	analyticsRepo := analyticsrepo.NewPostgres(dbConn)
	analyticsService := analyticsdomain.NewService(analyticsRepo)
	userRepo := userrepo.NewPostgres(dbConn)
	userService := userdomain.NewService(userRepo)
	todosRepo := todosrepo.NewPostgres(dbConn)
	todosService := todosdomain.NewService(todosRepo)
	handlers := handler.New(analyticsService, familyService, expensesService, todosService)

	router := httpserver.NewRouter(cfg, handlers, userService)
	server := httptest.NewServer(router)

	return &testEnv{server: server, authServer: authServer, db: dbConn}
}

func (e *testEnv) Close() {
	e.server.Close()
	e.authServer.Close()
	sqlDB, err := e.db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func newAuthServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		payload := map[string]interface{}{
			"id":    token,
			"email": token + "@example.com",
			"user_metadata": map[string]interface{}{
				"name":       "User " + token,
				"avatar_url": "https://example.com/avatar.png",
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
}

func cleanDB(dbConn *gorm.DB) error {
	return dbConn.WithContext(context.Background()).Exec(
		"TRUNCATE TABLE expense_tags, expenses, tags, family_members, families, user_profiles RESTART IDENTITY CASCADE",
	).Error
}

func requestJSON(t *testing.T, client *http.Client, method, url, token string, payload interface{}) (*http.Response, []byte) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return resp, respBody
}

func requestRaw(t *testing.T, client *http.Client, method, url string) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	return resp, respBody
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type authMeResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type familyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}

type familyMemberResponse struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
	Email     *string   `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
}

type expenseResponse struct {
	ID        string    `json:"id"`
	FamilyID  string    `json:"family_id"`
	UserID    string    `json:"user_id"`
	Date      string    `json:"date"`
	Amount    float64   `json:"amount"`
	Currency  string    `json:"currency"`
	Title     string    `json:"title"`
	TagIDs    []string  `json:"tag_ids"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type expenseListResponse struct {
	Items []expenseResponse `json:"items"`
	Total int64             `json:"total"`
}

type tagResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func TestE2EHealthAndAuth(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	resp, body := requestRaw(t, client, http.MethodGet, env.server.URL+"/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if strings.TrimSpace(string(body)) != "ok" {
		t.Fatalf("expected ok, got %q", string(body))
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/auth/me", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.StatusCode, string(body))
	}
	var errResp errorEnvelope
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errResp.Error.Code != "invalid_token" {
		t.Fatalf("expected invalid_token, got %q", errResp.Error.Code)
	}

	userID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/auth/me", userID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var me authMeResponse
	if err := json.Unmarshal(body, &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.ID != userID {
		t.Fatalf("expected id %s, got %q", userID, me.ID)
	}
	if me.Email != userID+"@example.com" {
		t.Fatalf("expected email, got %q", me.Email)
	}
}

func TestE2EFamilyFlow(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	user1 := "11111111-1111-1111-1111-111111111111"
	user2 := "22222222-2222-2222-2222-222222222222"

	resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/families", user1, map[string]string{
		"name": "Ivanovs",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var family familyResponse
	if err := json.Unmarshal(body, &family); err != nil {
		t.Fatalf("decode family: %v", err)
	}
	if family.ID == "" || family.Code == "" {
		t.Fatalf("expected family id and code")
	}

	resp, body = requestJSON(t, client, http.MethodPatch, env.server.URL+"/families/me", user1, map[string]string{
		"name": "Ivanovs 2",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/families/join", user2, map[string]string{
		"code": family.Code,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/families/me/members", user1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var members []familyMemberResponse
	if err := json.Unmarshal(body, &members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/families/leave", user1, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/families/leave", user2, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/families/leave", user1, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/families/me", user1, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestE2EExpensesAndTagsFlow(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	user1 := "11111111-1111-1111-1111-111111111111"

	resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/families", user1, map[string]string{
		"name": "Ivanovs",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", user1, map[string]interface{}{
		"date":     "2026-02-05",
		"amount":   12.5,
		"currency": "BYN",
		"title":    "Coffee",
		"tag_ids":  []string{"missing"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/tags", user1, map[string]string{
		"name": "Food",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var tag tagResponse
	if err := json.Unmarshal(body, &tag); err != nil {
		t.Fatalf("decode tag: %v", err)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", user1, map[string]interface{}{
		"date":     "2026-02-05",
		"amount":   12.5,
		"currency": "BYN",
		"title":    "Coffee",
		"tag_ids":  []string{tag.ID},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var expense expenseResponse
	if err := json.Unmarshal(body, &expense); err != nil {
		t.Fatalf("decode expense: %v", err)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/expenses?tag_id="+tag.ID, user1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var list expenseListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected total 1, got %d", list.Total)
	}

	resp, body = requestJSON(t, client, http.MethodPut, env.server.URL+"/expenses/"+expense.ID, user1, map[string]interface{}{
		"date":     "2026-02-05",
		"amount":   10.0,
		"currency": "USD",
		"title":    "Coffee 2",
		"tag_ids":  []string{tag.ID},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/expenses/"+expense.ID, user1, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/expenses/"+expense.ID, user1, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/tags/"+tag.ID, user1, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/tags/"+tag.ID, user1, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestE2EFamilyMembersManage(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	user1 := "33333333-3333-3333-3333-333333333333"
	user2 := "44444444-4444-4444-4444-444444444444"

	resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/families", user1, map[string]string{
		"name": "Ivanovs",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var family familyResponse
	if err := json.Unmarshal(body, &family); err != nil {
		t.Fatalf("decode family: %v", err)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/families/join", user2, map[string]string{
		"code": family.Code,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/families/me/members", user1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var members []familyMemberResponse
	if err := json.Unmarshal(body, &members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	for _, member := range members {
		if member.Email == nil || *member.Email == "" {
			t.Fatalf("expected email for member %s", member.UserID)
		}
		if member.AvatarURL == nil || *member.AvatarURL == "" {
			t.Fatalf("expected avatar_url for member %s", member.UserID)
		}
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/families/me/members/"+user1, user2, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/families/me/members/"+user1, user1, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/families/me/members/"+user2, user1, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/families/me/members/"+user2, user1, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/families/me/members", user1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, &members); err != nil {
		t.Fatalf("decode members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
}
