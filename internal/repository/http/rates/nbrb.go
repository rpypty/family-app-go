package rates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	ratesdomain "family-app-go/internal/domain/rates"
)

type NBRBClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

const excludedCurrencyCode = "XDR"

func NewNBRBClient(baseURL string, timeout time.Duration) (*NBRBClient, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.nbrb.by"
	}
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &NBRBClient{
		baseURL:    parsed,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *NBRBClient) ListCurrencies(ctx context.Context) ([]ratesdomain.Currency, error) {
	endpoint, err := c.resolveURL("/exrates/currencies")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nbrb list currencies: unexpected status %d", resp.StatusCode)
	}

	type row struct {
		Abbreviation string  `json:"Cur_Abbreviation"`
		Name         string  `json:"Cur_Name"`
		NameEng      string  `json:"Cur_Name_Eng"`
		Periodicity  int     `json:"Cur_Periodicity"`
		DateStart    *string `json:"Cur_DateStart"`
		DateEnd      *string `json:"Cur_DateEnd"`
	}

	var payload []row
	if err := decodeJSON(resp.Body, &payload); err != nil {
		return nil, err
	}

	now := dateOnlyUTC(time.Now())
	currencies := make([]ratesdomain.Currency, 0, len(payload))
	seen := make(map[string]struct{}, len(payload))
	for _, item := range payload {
		// Periodicity 0 means daily rates in NBRB API.
		if item.Periodicity != 0 {
			continue
		}
		if item.DateStart != nil {
			startDate, err := parseNBRBDate(*item.DateStart)
			if err == nil && dateOnlyUTC(startDate).After(now) {
				continue
			}
		}
		if item.DateEnd != nil {
			endDate, err := parseNBRBDate(*item.DateEnd)
			if err == nil && dateOnlyUTC(endDate).Before(now) {
				continue
			}
		}

		code := strings.ToUpper(strings.TrimSpace(item.Abbreviation))
		if len(code) != 3 {
			continue
		}
		if code == excludedCurrencyCode {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}

		name := strings.TrimSpace(item.NameEng)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name == "" {
			name = code
		}

		seen[code] = struct{}{}
		currencies = append(currencies, ratesdomain.Currency{
			Code: code,
			Name: name,
		})
	}

	return currencies, nil
}

func (c *NBRBClient) GetBYNRate(ctx context.Context, currency string, onDate time.Time) (ratesdomain.BYNRate, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return ratesdomain.BYNRate{}, ratesdomain.ErrInvalidCurrency
	}
	if currency == excludedCurrencyCode {
		return ratesdomain.BYNRate{}, ratesdomain.ErrRateNotAvailable
	}

	endpoint, err := c.resolveURL("/exrates/rates/" + url.PathEscape(currency))
	if err != nil {
		return ratesdomain.BYNRate{}, err
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return ratesdomain.BYNRate{}, err
	}
	query := parsedURL.Query()
	query.Set("parammode", "2")
	query.Set("periodicity", "0")
	query.Set("ondate", onDate.Format("2006-01-02"))
	parsedURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return ratesdomain.BYNRate{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ratesdomain.BYNRate{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ratesdomain.BYNRate{}, ratesdomain.ErrRateNotAvailable
	}
	if resp.StatusCode != http.StatusOK {
		return ratesdomain.BYNRate{}, fmt.Errorf("nbrb rate %s: unexpected status %d", currency, resp.StatusCode)
	}

	type payload struct {
		Abbreviation string   `json:"Cur_Abbreviation"`
		Scale        int      `json:"Cur_Scale"`
		Rate         *float64 `json:"Cur_OfficialRate"`
		Date         string   `json:"Date"`
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ratesdomain.BYNRate{}, err
	}

	body, err := decodeRatePayload(raw)
	if err != nil {
		return ratesdomain.BYNRate{}, err
	}
	if body == nil {
		return ratesdomain.BYNRate{}, ratesdomain.ErrRateNotAvailable
	}
	if body.Scale <= 0 {
		return ratesdomain.BYNRate{}, fmt.Errorf("invalid rate scale for %s", currency)
	}
	if body.Rate == nil {
		return ratesdomain.BYNRate{}, ratesdomain.ErrRateNotAvailable
	}

	code := strings.ToUpper(strings.TrimSpace(body.Abbreviation))
	if code == "" {
		code = currency
	}
	date := onDate
	if parsedDate, err := parseNBRBDate(body.Date); err == nil {
		date = parsedDate
	}
	if date.IsZero() {
		date = onDate
	}

	return ratesdomain.BYNRate{
		Code:  code,
		Date:  dateOnlyUTC(date),
		Scale: body.Scale,
		Rate:  *body.Rate,
	}, nil
}

func (c *NBRBClient) resolveURL(path string) (string, error) {
	relative, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	return c.baseURL.ResolveReference(relative).String(), nil
}

func decodeJSON(body io.Reader, dst interface{}) error {
	dec := json.NewDecoder(body)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid json payload")
	}
	return nil
}

func decodeRatePayload(raw []byte) (*struct {
	Abbreviation string   `json:"Cur_Abbreviation"`
	Scale        int      `json:"Cur_Scale"`
	Rate         *float64 `json:"Cur_OfficialRate"`
	Date         string   `json:"Date"`
}, error) {
	type payload struct {
		Abbreviation string   `json:"Cur_Abbreviation"`
		Scale        int      `json:"Cur_Scale"`
		Rate         *float64 `json:"Cur_OfficialRate"`
		Date         string   `json:"Date"`
	}

	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	switch trimmed[0] {
	case '{':
		var item payload
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		return &struct {
			Abbreviation string   `json:"Cur_Abbreviation"`
			Scale        int      `json:"Cur_Scale"`
			Rate         *float64 `json:"Cur_OfficialRate"`
			Date         string   `json:"Date"`
		}{
			Abbreviation: item.Abbreviation,
			Scale:        item.Scale,
			Rate:         item.Rate,
			Date:         item.Date,
		}, nil
	case '[':
		var items []payload
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
		if len(items) == 0 {
			return nil, nil
		}
		item := items[0]
		return &struct {
			Abbreviation string   `json:"Cur_Abbreviation"`
			Scale        int      `json:"Cur_Scale"`
			Rate         *float64 `json:"Cur_OfficialRate"`
			Date         string   `json:"Date"`
		}{
			Abbreviation: item.Abbreviation,
			Scale:        item.Scale,
			Rate:         item.Rate,
			Date:         item.Date,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected nbrb response payload")
	}
}

func dateOnlyUTC(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func parseNBRBDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}

	layouts := []string{
		"2006-01-02T15:04:05",
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if layout == "2006-01-02T15:04:05" {
			parsed, err := time.ParseInLocation(layout, value, time.UTC)
			if err == nil {
				return parsed, nil
			}
			continue
		}
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid nbrb date: %s", value)
}
