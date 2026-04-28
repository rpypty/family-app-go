package rates

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidCurrency  = errors.New("invalid currency")
	ErrRateNotAvailable = errors.New("rate not available")
)

type Currency struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Icon   string `json:"icon"`
	Symbol string `json:"symbol"`
}

type Quote struct {
	From   string
	To     string
	Rate   float64
	Date   time.Time
	Source string
}

type BYNRate struct {
	Code  string
	Date  time.Time
	Scale int
	Rate  float64
}

type Provider interface {
	ListCurrencies(ctx context.Context) ([]Currency, error)
	GetBYNRate(ctx context.Context, currency string, onDate time.Time) (BYNRate, error)
}
