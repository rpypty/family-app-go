package rates

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	currencies []Currency
	rates      map[string]BYNRate
}

func (f *fakeProvider) ListCurrencies(context.Context) ([]Currency, error) {
	result := make([]Currency, len(f.currencies))
	copy(result, f.currencies)
	return result, nil
}

func (f *fakeProvider) GetBYNRate(_ context.Context, currency string, onDate time.Time) (BYNRate, error) {
	key := currency + "|" + onDate.Format("2006-01-02")
	rate, ok := f.rates[key]
	if !ok {
		return BYNRate{}, ErrRateNotAvailable
	}
	return rate, nil
}

func TestGetRateSameCurrency(t *testing.T) {
	svc := NewService(&fakeProvider{}, Config{})
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	quote, err := svc.GetRate(context.Background(), "usd", "USD", date)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if quote.Rate != 1 {
		t.Fatalf("expected rate 1, got %v", quote.Rate)
	}
	if quote.Source != "identity" {
		t.Fatalf("expected identity source, got %q", quote.Source)
	}
}

func TestGetRateUsesFallbackDate(t *testing.T) {
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	fallbackDate := date.AddDate(0, 0, -1)

	svc := NewService(&fakeProvider{rates: map[string]BYNRate{
		"USD|2026-03-04": {Code: "USD", Date: fallbackDate, Scale: 1, Rate: 3.2},
		"RUB|2026-03-04": {Code: "RUB", Date: fallbackDate, Scale: 100, Rate: 3.6},
	}}, Config{FallbackDays: 3})

	quote, err := svc.GetRate(context.Background(), "USD", "RUB", date)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if quote.Date.Format("2006-01-02") != "2026-03-04" {
		t.Fatalf("expected fallback date 2026-03-04, got %s", quote.Date.Format("2006-01-02"))
	}
	expected := 3.2 / (3.6 / 100)
	if quote.Rate != expected {
		t.Fatalf("expected rate %v, got %v", expected, quote.Rate)
	}
}

func TestGetRateNotAvailable(t *testing.T) {
	svc := NewService(&fakeProvider{}, Config{FallbackDays: 1})
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	_, err := svc.GetRate(context.Background(), "USD", "RUB", date)
	if !errors.Is(err, ErrRateNotAvailable) {
		t.Fatalf("expected ErrRateNotAvailable, got %v", err)
	}
}

func TestListCurrenciesAddsBYN(t *testing.T) {
	svc := NewService(&fakeProvider{currencies: []Currency{{Code: "USD", Name: "US Dollar"}}}, Config{})

	currencies, err := svc.ListCurrencies(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(currencies) != 2 {
		t.Fatalf("expected 2 currencies, got %d", len(currencies))
	}
	if currencies[0].Code != "BYN" && currencies[1].Code != "BYN" {
		t.Fatalf("expected BYN in list, got %+v", currencies)
	}
	for _, currency := range currencies {
		if currency.Code == "BYN" && currency.Symbol != "ƃ" {
			t.Fatalf("expected BYN symbol ƃ, got %+v", currency)
		}
	}
}
