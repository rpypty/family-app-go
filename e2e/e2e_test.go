//go:build e2e
// +build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
	ratesdomain "family-app-go/internal/domain/rates"
	todosdomain "family-app-go/internal/domain/todos"
	userdomain "family-app-go/internal/domain/user"
	inmemoryrepo "family-app-go/internal/repository/inmemory"
	analyticsrepo "family-app-go/internal/repository/postgres/analytics"
	expensesrepo "family-app-go/internal/repository/postgres/expenses"
	familyrepo "family-app-go/internal/repository/postgres/family"
	todosrepo "family-app-go/internal/repository/postgres/todos"
	userrepo "family-app-go/internal/repository/postgres/user"
	"family-app-go/internal/transport/httpserver"
	"family-app-go/internal/transport/httpserver/handler"
	"family-app-go/pkg/logger"
	"gorm.io/gorm"
)

type testEnv struct {
	server     *httptest.Server
	authServer *httptest.Server
	db         *gorm.DB
}

type e2eRatesProvider struct{}

func (e2eRatesProvider) ListCurrencies(_ context.Context) ([]ratesdomain.Currency, error) {
	return []ratesdomain.Currency{
		{Code: "BYN", Name: "Belarusian Ruble", Icon: "🇧🇾"},
		{Code: "USD", Name: "US Dollar", Icon: "🇺🇸"},
		{Code: "EUR", Name: "Euro", Icon: "🇪🇺"},
		{Code: "RUB", Name: "Russian Ruble", Icon: "🇷🇺"},
	}, nil
}

func (e2eRatesProvider) GetBYNRate(_ context.Context, currency string, onDate time.Time) (ratesdomain.BYNRate, error) {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "USD":
		return ratesdomain.BYNRate{Code: "USD", Date: onDate, Scale: 1, Rate: 3.2}, nil
	case "EUR":
		return ratesdomain.BYNRate{Code: "EUR", Date: onDate, Scale: 1, Rate: 3.5}, nil
	case "RUB":
		return ratesdomain.BYNRate{Code: "RUB", Date: onDate, Scale: 100, Rate: 3.6}, nil
	default:
		return ratesdomain.BYNRate{}, ratesdomain.ErrRateNotAvailable
	}
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
		TopCategories: config.TopCategoriesConfig{
			Enabled:       true,
			LookbackDays:  30,
			DBReadLimit:   1000,
			MinRecords:    1,
			ResponseCount: 5,
			CacheTTL:      time.Minute,
		},
		Supabase: config.SupabaseConfig{
			URL:            authServer.URL,
			PublishableKey: "test-key",
			AuthTimeout:    2 * time.Second,
		},
	}

	log := logger.New(io.Discard, slog.LevelError, "text")

	dbConn, err := db.NewPostgres(log, cfg.DB)
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
	familyService := familydomain.NewServiceWithCache(familyRepo, inmemoryrepo.NewInMemoryFamilyCache())
	expensesRepo := expensesrepo.NewPostgres(dbConn)
	ratesService := ratesdomain.NewService(e2eRatesProvider{}, ratesdomain.Config{
		RateCacheTTL:       time.Minute,
		CurrenciesCacheTTL: time.Minute,
		FallbackDays:       0,
	})
	expensesService := expensesdomain.NewServiceWithDependencies(expensesRepo, inmemoryrepo.NewInMemoryCategoriesCache(), ratesService)
	analyticsRepo := analyticsrepo.NewPostgres(dbConn)
	analyticsService := analyticsdomain.NewServiceWithTopCategoriesConfig(analyticsRepo, analyticsdomain.TopCategoriesConfig{
		Enabled:       cfg.TopCategories.Enabled,
		LookbackDays:  cfg.TopCategories.LookbackDays,
		DBReadLimit:   cfg.TopCategories.DBReadLimit,
		MinRecords:    cfg.TopCategories.MinRecords,
		ResponseCount: cfg.TopCategories.ResponseCount,
		CacheTTL:      cfg.TopCategories.CacheTTL,
	})
	userRepo := userrepo.NewPostgres(dbConn)
	userService := userdomain.NewService(userRepo)
	todosRepo := todosrepo.NewPostgres(dbConn)
	todosService := todosdomain.NewService(todosRepo)
	handlers := handler.New(analyticsService, familyService, expensesService, ratesService, todosService, nil, nil, log)

	router := httpserver.NewRouter(cfg, handlers, userService, log)
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
		"TRUNCATE TABLE expense_categories, expenses, categories, family_members, families, user_profiles RESTART IDENTITY CASCADE",
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
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Code            string    `json:"code"`
	OwnerID         string    `json:"owner_id"`
	DefaultCurrency string    `json:"default_currency"`
	CreatedAt       time.Time `json:"created_at"`
}

type familyMemberResponse struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
	Email     *string   `json:"email"`
	AvatarURL *string   `json:"avatar_url"`
}

type expenseResponse struct {
	ID           string    `json:"id"`
	FamilyID     string    `json:"family_id"`
	UserID       string    `json:"user_id"`
	Date         string    `json:"date"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	BaseCurrency *string   `json:"base_currency"`
	ExchangeRate *float64  `json:"exchange_rate"`
	AmountInBase *float64  `json:"amount_in_base"`
	RateDate     *string   `json:"rate_date"`
	RateSource   *string   `json:"rate_source"`
	Title        string    `json:"title"`
	CategoryIDs  []string  `json:"category_ids"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type expenseListResponse struct {
	Items []expenseResponse `json:"items"`
	Total int64             `json:"total"`
}

type categoryResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     *string   `json:"color"`
	Emoji     *string   `json:"emoji"`
	CreatedAt time.Time `json:"created_at"`
}

type analyticsByCategoryRowResponse struct {
	CategoryID   string  `json:"category_id"`
	CategoryName string  `json:"category_name"`
	Total        float64 `json:"total"`
	Count        int64   `json:"count"`
}

type topCategoriesResponse struct {
	Status string                           `json:"status"`
	Items  []analyticsByCategoryRowResponse `json:"items"`
}

type analyticsSummaryResponse struct {
	TotalAmount float64 `json:"total_amount"`
	Currency    string  `json:"currency"`
	Count       int64   `json:"count"`
	AvgPerDay   float64 `json:"avg_per_day"`
	From        string  `json:"from"`
	To          string  `json:"to"`
}

type exchangeRateResponse struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Date   string  `json:"date"`
	Rate   float64 `json:"rate"`
	Source string  `json:"source"`
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

func TestE2EExpensesAndCategoriesFlow(t *testing.T) {
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
		"date":         "2026-02-05",
		"amount":       12.5,
		"currency":     "BYN",
		"title":        "Coffee",
		"category_ids": []string{"missing"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/categories", user1, map[string]interface{}{
		"name":  "Food",
		"color": "#AABBCC",
		"emoji": "🙂",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var category categoryResponse
	if err := json.Unmarshal(body, &category); err != nil {
		t.Fatalf("decode category: %v", err)
	}
	if category.Color == nil || *category.Color != "#aabbcc" {
		t.Fatalf("expected normalized color, got %+v", category.Color)
	}
	if category.Emoji == nil || *category.Emoji != "🙂" {
		t.Fatalf("expected emoji, got %+v", category.Emoji)
	}

	resp, body = requestJSON(t, client, http.MethodPatch, env.server.URL+"/categories/"+category.ID, user1, map[string]interface{}{
		"name":  "Food Updated",
		"color": "#00FF11",
		"emoji": "❤️",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, &category); err != nil {
		t.Fatalf("decode updated category: %v", err)
	}
	if category.Name != "Food Updated" {
		t.Fatalf("expected updated name, got %q", category.Name)
	}
	if category.Color == nil || *category.Color != "#00ff11" {
		t.Fatalf("expected normalized color, got %+v", category.Color)
	}
	if category.Emoji == nil || *category.Emoji != "❤️" {
		t.Fatalf("expected emoji, got %+v", category.Emoji)
	}

	resp, body = requestJSON(t, client, http.MethodPatch, env.server.URL+"/categories/"+category.ID, user1, map[string]interface{}{
		"name":  "Food Updated",
		"color": nil,
		"emoji": nil,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, &category); err != nil {
		t.Fatalf("decode cleared category: %v", err)
	}
	if category.Color != nil {
		t.Fatalf("expected nil color, got %+v", category.Color)
	}
	if category.Emoji != nil {
		t.Fatalf("expected nil emoji, got %+v", category.Emoji)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/categories", user1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var categories []categoryResponse
	if err := json.Unmarshal(body, &categories); err != nil {
		t.Fatalf("decode categories list: %v", err)
	}
	if len(categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(categories))
	}
	if categories[0].Color != nil || categories[0].Emoji != nil {
		t.Fatalf("expected cleared color/emoji, got color=%+v emoji=%+v", categories[0].Color, categories[0].Emoji)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/categories", user1, map[string]interface{}{
		"name":  "Invalid Color",
		"color": "#12GG34",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid color, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/categories", user1, map[string]interface{}{
		"name":  "Invalid Emoji",
		"emoji": "ab",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid emoji, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", user1, map[string]interface{}{
		"date":         "2026-02-05",
		"amount":       12.5,
		"currency":     "BYN",
		"title":        "Coffee",
		"category_ids": []string{category.ID},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var expense expenseResponse
	if err := json.Unmarshal(body, &expense); err != nil {
		t.Fatalf("decode expense: %v", err)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/expenses?category_id="+category.ID, user1, nil)
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
		"date":         "2026-02-05",
		"amount":       10.0,
		"currency":     "USD",
		"title":        "Coffee 2",
		"category_ids": []string{category.ID},
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

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/categories/"+category.ID, user1, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodDelete, env.server.URL+"/categories/"+category.ID, user1, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestE2ERatesEndpoints(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	user := "77777777-7777-7777-7777-777777777777"

	resp, body := requestJSON(t, client, http.MethodGet, env.server.URL+"/currencies", user, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var currencies []struct {
		Code string `json:"code"`
		Name string `json:"name"`
		Icon string `json:"icon"`
	}
	if err := json.Unmarshal(body, &currencies); err != nil {
		t.Fatalf("decode currencies: %v", err)
	}
	if len(currencies) == 0 {
		t.Fatalf("expected non-empty currencies")
	}
	hasBYN := false
	hasUSD := false
	for _, item := range currencies {
		if item.Code == "BYN" {
			hasBYN = true
			if item.Icon == "" {
				t.Fatalf("expected BYN icon")
			}
		}
		if item.Code == "USD" {
			hasUSD = true
			if item.Icon == "" {
				t.Fatalf("expected USD icon")
			}
		}
	}
	if !hasBYN || !hasUSD {
		t.Fatalf("expected BYN and USD in currencies, got %+v", currencies)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/exchange-rates?from=USD&to=BYN&date=2026-02-10", user, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var rate exchangeRateResponse
	if err := json.Unmarshal(body, &rate); err != nil {
		t.Fatalf("decode exchange rate: %v", err)
	}
	if rate.From != "USD" || rate.To != "BYN" || rate.Date != "2026-02-10" {
		t.Fatalf("unexpected rate payload: %+v", rate)
	}
	if rate.Rate != 3.2 {
		t.Fatalf("expected rate 3.2, got %v", rate.Rate)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/exchange-rates?from=GBP&to=BYN&date=2026-02-10", user, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", resp.StatusCode, string(body))
	}
	var errResp errorEnvelope
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != "rate_not_available" {
		t.Fatalf("expected rate_not_available, got %q", errResp.Error.Code)
	}
}

func TestE2EExpenseConversionAndAnalyticsModes(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	user := "88888888-8888-8888-8888-888888888888"

	resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/families", user, map[string]string{
		"name": "Conversion Family",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var family familyResponse
	if err := json.Unmarshal(body, &family); err != nil {
		t.Fatalf("decode family: %v", err)
	}
	if family.DefaultCurrency != "USD" {
		t.Fatalf("expected default currency USD, got %q", family.DefaultCurrency)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", user, map[string]interface{}{
		"date":     "2026-02-10",
		"amount":   32.0,
		"currency": "BYN",
		"title":    "BYN expense",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var bynExpense expenseResponse
	if err := json.Unmarshal(body, &bynExpense); err != nil {
		t.Fatalf("decode BYN expense: %v", err)
	}
	if bynExpense.BaseCurrency == nil || *bynExpense.BaseCurrency != "USD" {
		t.Fatalf("expected base currency USD, got %+v", bynExpense.BaseCurrency)
	}
	if bynExpense.ExchangeRate == nil || *bynExpense.ExchangeRate != 0.3125 {
		t.Fatalf("expected exchange rate 0.3125, got %+v", bynExpense.ExchangeRate)
	}
	if bynExpense.AmountInBase == nil || *bynExpense.AmountInBase != 10 {
		t.Fatalf("expected amount_in_base 10, got %+v", bynExpense.AmountInBase)
	}
	if bynExpense.RateSource == nil || *bynExpense.RateSource != "nbrb" {
		t.Fatalf("expected rate source nbrb, got %+v", bynExpense.RateSource)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", user, map[string]interface{}{
		"date":     "2026-02-10",
		"amount":   5.0,
		"currency": "USD",
		"title":    "USD expense",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/expenses?currency=USD", user, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var expensesList expenseListResponse
	if err := json.Unmarshal(body, &expensesList); err != nil {
		t.Fatalf("decode expenses list: %v", err)
	}
	if expensesList.Total != 1 || len(expensesList.Items) != 1 || expensesList.Items[0].Currency != "USD" {
		t.Fatalf("expected single USD expense, got %+v", expensesList)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/analytics/summary?from=2026-02-10&to=2026-02-10", user, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var summary analyticsSummaryResponse
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if summary.Currency != "USD" {
		t.Fatalf("expected summary currency USD, got %q", summary.Currency)
	}
	if summary.TotalAmount != 15 {
		t.Fatalf("expected total 15, got %v", summary.TotalAmount)
	}

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/analytics/summary?from=2026-02-10&to=2026-02-10&currency=BYN", user, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode BYN summary: %v", err)
	}
	if summary.Currency != "BYN" {
		t.Fatalf("expected summary currency BYN, got %q", summary.Currency)
	}
	if summary.TotalAmount != 32 {
		t.Fatalf("expected total 32 for BYN filter, got %v", summary.TotalAmount)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", user, map[string]interface{}{
		"date":     "2026-02-10",
		"amount":   10.0,
		"currency": "GBP",
		"title":    "GBP expense",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestE2EDefaultCurrencyLocked(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	user := "99999999-9999-9999-9999-999999999999"

	resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/families", user, map[string]string{
		"name": "Locked Currency Family",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}

	resp, body = requestJSON(t, client, http.MethodPatch, env.server.URL+"/families/me", user, map[string]string{
		"default_currency": "BYN",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.StatusCode, string(body))
	}
	var errResp errorEnvelope
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Error.Code != "base_currency_locked" {
		t.Fatalf("expected base_currency_locked, got %q", errResp.Error.Code)
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

func TestE2ETopCategoriesByFamily(t *testing.T) {
	env := setupE2E(t)
	defer env.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	user1 := "55555555-5555-5555-5555-555555555555"
	user2 := "66666666-6666-6666-6666-666666666666"

	resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/families", user1, map[string]string{
		"name": "Analytics Family",
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

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/categories", user1, map[string]interface{}{
		"name": "Food",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var food categoryResponse
	if err := json.Unmarshal(body, &food); err != nil {
		t.Fatalf("decode food category: %v", err)
	}

	resp, body = requestJSON(t, client, http.MethodPost, env.server.URL+"/categories", user1, map[string]interface{}{
		"name": "Transport",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
	}
	var transport categoryResponse
	if err := json.Unmarshal(body, &transport); err != nil {
		t.Fatalf("decode transport category: %v", err)
	}

	createExpense := func(userID, title string, amount float64, categoryID string) {
		t.Helper()
		resp, body := requestJSON(t, client, http.MethodPost, env.server.URL+"/expenses", userID, map[string]interface{}{
			"date":         "2026-02-10",
			"amount":       amount,
			"currency":     "USD",
			"title":        title,
			"category_ids": []string{categoryID},
		})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(body))
		}
	}

	createExpense(user1, "Lunch", 10, food.ID)
	createExpense(user1, "Dinner", 15, food.ID)
	createExpense(user1, "Taxi", 40, transport.ID)

	createExpense(user2, "Food shared #1", 100, food.ID)
	createExpense(user2, "Food shared #2", 200, food.ID)

	resp, body = requestJSON(t, client, http.MethodGet, env.server.URL+"/top_categories", user1, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
	var result topCategoriesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode top categories response: %v", err)
	}
	if result.Status != "OK" {
		t.Fatalf("expected status OK, got %q", result.Status)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Items))
	}
	if result.Items[0].CategoryID != food.ID || result.Items[0].Count != 4 {
		t.Fatalf("expected family-aggregated food first, got %+v", result.Items[0])
	}
	if result.Items[1].CategoryID != transport.ID || result.Items[1].Count != 1 {
		t.Fatalf("expected transport second, got %+v", result.Items[1])
	}
}
