package expenses

import (
	"errors"
	"net/http"
	"strings"

	ratesdomain "family-app-go/internal/domain/rates"
)

type currencyResponse struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

type exchangeRateResponse struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Date   string  `json:"date"`
	Rate   float64 `json:"rate"`
	Source string  `json:"source"`
}

func (h *Handlers) ListCurrencies(w http.ResponseWriter, r *http.Request) {
	currencies, err := h.Rates.ListCurrencies(r.Context())
	if err != nil {
		h.log.InternalError("rates.list_currencies: list currencies failed", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]currencyResponse, 0, len(currencies))
	for _, currency := range currencies {
		response = append(response, currencyResponse{
			Code: currency.Code,
			Name: currency.Name,
			Icon: currency.Icon,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) GetExchangeRate(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	from := strings.TrimSpace(query.Get("from"))
	to := strings.TrimSpace(query.Get("to"))
	date, err := parseDateRequired(query.Get("date"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "date is required")
		return
	}

	quote, err := h.Rates.GetRate(r.Context(), from, to, date)
	if err != nil {
		switch {
		case errors.Is(err, ratesdomain.ErrInvalidCurrency):
			writeError(w, http.StatusBadRequest, "invalid_request", "from and to must be 3-letter currency codes")
		case errors.Is(err, ratesdomain.ErrRateNotAvailable):
			writeError(w, http.StatusNotFound, "rate_not_available", "rate is not available for selected date")
		default:
			h.log.InternalError("rates.get_exchange_rate: get rate failed", err, "from", from, "to", to, "date", date.Format("2006-01-02"))
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, exchangeRateResponse{
		From:   quote.From,
		To:     quote.To,
		Date:   quote.Date.Format("2006-01-02"),
		Rate:   quote.Rate,
		Source: quote.Source,
	})
}
