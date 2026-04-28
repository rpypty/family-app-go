package rates

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Config struct {
	RateCacheTTL       time.Duration
	CurrenciesCacheTTL time.Duration
	FallbackDays       int
}

type Service struct {
	provider Provider

	rateCacheTTL       time.Duration
	currenciesCacheTTL time.Duration
	fallbackDays       int

	rateMu    sync.RWMutex
	rateCache map[string]cacheItem[Quote]

	currenciesMu       sync.RWMutex
	currenciesCache    []Currency
	currenciesExpireAt time.Time
}

type cacheItem[T any] struct {
	value     T
	expiresAt time.Time
}

func NewService(provider Provider, cfg Config) *Service {
	if cfg.RateCacheTTL <= 0 {
		cfg.RateCacheTTL = 12 * time.Hour
	}
	if cfg.CurrenciesCacheTTL <= 0 {
		cfg.CurrenciesCacheTTL = 24 * time.Hour
	}
	if cfg.FallbackDays < 0 {
		cfg.FallbackDays = 0
	}

	return &Service{
		provider:           provider,
		rateCacheTTL:       cfg.RateCacheTTL,
		currenciesCacheTTL: cfg.CurrenciesCacheTTL,
		fallbackDays:       cfg.FallbackDays,
		rateCache:          make(map[string]cacheItem[Quote]),
	}
}

func (s *Service) ListCurrencies(ctx context.Context) ([]Currency, error) {
	now := time.Now()

	s.currenciesMu.RLock()
	if len(s.currenciesCache) > 0 && s.currenciesExpireAt.After(now) {
		cached := cloneCurrencies(s.currenciesCache)
		s.currenciesMu.RUnlock()
		return cached, nil
	}
	s.currenciesMu.RUnlock()

	currencies, err := s.provider.ListCurrencies(ctx)
	if err != nil {
		return nil, err
	}

	// Ensure base currency is always available.
	if !hasCurrency(currencies, "BYN") {
		currencies = append([]Currency{{Code: "BYN", Name: "Belarusian Ruble", Icon: "🇧🇾", Symbol: "ƃ"}}, currencies...)
	}

	s.currenciesMu.Lock()
	s.currenciesCache = cloneCurrencies(currencies)
	s.currenciesExpireAt = now.Add(s.currenciesCacheTTL)
	s.currenciesMu.Unlock()

	return cloneCurrencies(currencies), nil
}

func (s *Service) GetRate(ctx context.Context, from, to string, onDate time.Time) (Quote, error) {
	fromCode, err := normalizeCurrency(from)
	if err != nil {
		return Quote{}, err
	}
	toCode, err := normalizeCurrency(to)
	if err != nil {
		return Quote{}, err
	}

	if onDate.IsZero() {
		return Quote{}, fmt.Errorf("date is required")
	}
	onDate = dateOnlyUTC(onDate)

	if fromCode == toCode {
		return Quote{
			From:   fromCode,
			To:     toCode,
			Rate:   1,
			Date:   onDate,
			Source: "identity",
		}, nil
	}

	cacheKey := rateCacheKey(fromCode, toCode, onDate)
	if cached, ok := s.getQuoteFromCache(cacheKey, time.Now()); ok {
		return cached, nil
	}

	var lastErr error
	for offset := 0; offset <= s.fallbackDays; offset++ {
		rateDate := onDate.AddDate(0, 0, -offset)

		fromBYN, err := s.getBYNPerUnitOnDate(ctx, fromCode, rateDate)
		if err != nil {
			if err == ErrRateNotAvailable {
				lastErr = err
				continue
			}
			return Quote{}, err
		}

		toBYN, err := s.getBYNPerUnitOnDate(ctx, toCode, rateDate)
		if err != nil {
			if err == ErrRateNotAvailable {
				lastErr = err
				continue
			}
			return Quote{}, err
		}

		quote := Quote{
			From:   fromCode,
			To:     toCode,
			Rate:   fromBYN / toBYN,
			Date:   rateDate,
			Source: "nbrb",
		}

		s.setQuoteCache(cacheKey, quote, time.Now().Add(s.rateCacheTTL))
		return quote, nil
	}

	if lastErr != nil {
		return Quote{}, lastErr
	}
	return Quote{}, ErrRateNotAvailable
}

func (s *Service) getBYNPerUnitOnDate(ctx context.Context, currency string, onDate time.Time) (float64, error) {
	if currency == "BYN" {
		return 1, nil
	}

	rate, err := s.provider.GetBYNRate(ctx, currency, onDate)
	if err != nil {
		return 0, err
	}
	if rate.Scale <= 0 {
		return 0, fmt.Errorf("invalid scale for currency %s", currency)
	}
	return rate.Rate / float64(rate.Scale), nil
}

func (s *Service) getQuoteFromCache(key string, now time.Time) (Quote, bool) {
	s.rateMu.RLock()
	item, ok := s.rateCache[key]
	s.rateMu.RUnlock()
	if !ok {
		return Quote{}, false
	}

	if !item.expiresAt.After(now) {
		s.rateMu.Lock()
		item, ok = s.rateCache[key]
		if ok && !item.expiresAt.After(now) {
			delete(s.rateCache, key)
		}
		s.rateMu.Unlock()
		return Quote{}, false
	}

	return item.value, true
}

func (s *Service) setQuoteCache(key string, quote Quote, expiresAt time.Time) {
	s.rateMu.Lock()
	s.rateCache[key] = cacheItem[Quote]{value: quote, expiresAt: expiresAt}
	s.rateMu.Unlock()
}

func normalizeCurrency(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) != 3 {
		return "", ErrInvalidCurrency
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 'A' || value[i] > 'Z' {
			return "", ErrInvalidCurrency
		}
	}
	return value, nil
}

func hasCurrency(currencies []Currency, code string) bool {
	for i := range currencies {
		if strings.EqualFold(currencies[i].Code, code) {
			return true
		}
	}
	return false
}

func cloneCurrencies(currencies []Currency) []Currency {
	if currencies == nil {
		return nil
	}
	cloned := make([]Currency, len(currencies))
	copy(cloned, currencies)
	return cloned
}

func rateCacheKey(from, to string, date time.Time) string {
	return from + "|" + to + "|" + date.Format("2006-01-02")
}

func dateOnlyUTC(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}
