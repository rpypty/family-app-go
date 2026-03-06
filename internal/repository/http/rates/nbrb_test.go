package rates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	ratesdomain "family-app-go/internal/domain/rates"
)

func TestListCurrenciesFiltersByPeriodicityAndActiveDates(t *testing.T) {
	now := time.Now().UTC()
	futureStart := now.AddDate(0, 0, 10).Format(time.RFC3339)
	pastEnd := now.AddDate(0, 0, -10).Format(time.RFC3339)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exrates/currencies" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"Cur_Abbreviation":"USD","Cur_Name":"Доллар США","Cur_Name_Eng":"US Dollar","Cur_Periodicity":0,"Cur_DateStart":"2000-01-01T00:00:00","Cur_DateEnd":null,"Extra":"value"},
			{"Cur_Abbreviation":"XDR","Cur_Name":"XDR","Cur_Name_Eng":"XDR","Cur_Periodicity":0,"Cur_DateStart":"2000-01-01T00:00:00","Cur_DateEnd":null},
			{"Cur_Abbreviation":"EUR","Cur_Name":"Евро","Cur_Name_Eng":"Euro","Cur_Periodicity":1,"Cur_DateStart":"2000-01-01T00:00:00","Cur_DateEnd":null},
			{"Cur_Abbreviation":"OLD","Cur_Name":"Old","Cur_Name_Eng":"Old","Cur_Periodicity":0,"Cur_DateStart":"2000-01-01T00:00:00","Cur_DateEnd":"` + pastEnd + `"},
			{"Cur_Abbreviation":"FUT","Cur_Name":"Future","Cur_Name_Eng":"Future","Cur_Periodicity":0,"Cur_DateStart":"` + futureStart + `","Cur_DateEnd":null}
		]`))
	}))
	defer server.Close()

	client, err := NewNBRBClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	currencies, err := client.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("list currencies: %v", err)
	}
	if len(currencies) != 1 {
		t.Fatalf("expected 1 currency, got %d: %+v", len(currencies), currencies)
	}
	if currencies[0].Code != "USD" || currencies[0].Name != "US Dollar" {
		t.Fatalf("unexpected currency %+v", currencies[0])
	}
}

func TestGetBYNRateParsesObjectPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exrates/rates/USD" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("parammode") != "2" {
			t.Fatalf("expected parammode=2, got %q", query.Get("parammode"))
		}
		if query.Get("periodicity") != "0" {
			t.Fatalf("expected periodicity=0, got %q", query.Get("periodicity"))
		}
		if query.Get("ondate") != "2026-03-05" {
			t.Fatalf("expected ondate=2026-03-05, got %q", query.Get("ondate"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Cur_Abbreviation":"USD","Cur_Scale":1,"Cur_OfficialRate":3.2,"Date":"2026-03-05T00:00:00","Cur_ID":145}`))
	}))
	defer server.Close()

	client, err := NewNBRBClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	rate, err := client.GetBYNRate(context.Background(), "USD", time.Date(2026, 3, 5, 12, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("get rate: %v", err)
	}
	if rate.Code != "USD" || rate.Scale != 1 || rate.Rate != 3.2 {
		t.Fatalf("unexpected rate %+v", rate)
	}
	if rate.Date.Format("2006-01-02") != "2026-03-05" {
		t.Fatalf("unexpected date %s", rate.Date.Format("2006-01-02"))
	}
}

func TestGetBYNRateReturnsNotAvailableOnEmptyArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := NewNBRBClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.GetBYNRate(context.Background(), "USD", time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ratesdomain.ErrRateNotAvailable) {
		t.Fatalf("expected ErrRateNotAvailable, got %v", err)
	}
}

func TestGetBYNRateReturnsNotAvailableOnNullRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Cur_Abbreviation":"USD","Cur_Scale":1,"Cur_OfficialRate":null,"Date":"2026-03-05T00:00:00"}`))
	}))
	defer server.Close()

	client, err := NewNBRBClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.GetBYNRate(context.Background(), "USD", time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ratesdomain.ErrRateNotAvailable) {
		t.Fatalf("expected ErrRateNotAvailable, got %v", err)
	}
}

func TestGetBYNRateSkipsExcludedCurrency(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewNBRBClient(server.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.GetBYNRate(context.Background(), "XDR", time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ratesdomain.ErrRateNotAvailable) {
		t.Fatalf("expected ErrRateNotAvailable, got %v", err)
	}
	if called {
		t.Fatalf("expected no external request for excluded currency")
	}
}
