package expenses

import (
	"errors"
	"net/http"
	"strings"
	"time"

	analyticsdomain "family-app-go/internal/domain/analytics"
	familydomain "family-app-go/internal/domain/family"
	"family-app-go/internal/transport/httpserver/middleware"
)

func (h *Handlers) AnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("analytics.summary: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("analytics.summary: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	from, err := parseDateRequired(query.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from is required")
		return
	}
	to, err := parseDateRequired(query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "to is required")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "invalid_request", "from must be <= to")
		return
	}

	currency := strings.TrimSpace(query.Get("currency"))
	categoryIDs := parseCSV(query.Get("category_ids"))
	_, err = normalizeTimezone(query.Get("timezone"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid timezone")
		return
	}

	result, err := h.Analytics.Summary(r.Context(), family.ID, analyticsdomain.SummaryFilter{
		From:        from,
		To:          to,
		Currency:    currency,
		CategoryIDs: categoryIDs,
	})
	if err != nil {
		h.log.InternalError("analytics.summary: build summary failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_amount": result.TotalAmount,
		"currency":     currency,
		"count":        result.Count,
		"avg_per_day":  result.AvgPerDay,
		"from":         from.Format("2006-01-02"),
		"to":           to.Format("2006-01-02"),
	})
}

func (h *Handlers) AnalyticsTimeseries(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("analytics.timeseries: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("analytics.timeseries: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	from, err := parseDateRequired(query.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from is required")
		return
	}
	to, err := parseDateRequired(query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "to is required")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "invalid_request", "from must be <= to")
		return
	}

	groupBy := strings.ToLower(strings.TrimSpace(query.Get("group_by")))
	if groupBy != "day" && groupBy != "week" {
		writeError(w, http.StatusBadRequest, "invalid_request", "group_by must be day or week")
		return
	}

	currency := strings.TrimSpace(query.Get("currency"))
	categoryIDs := parseCSV(query.Get("category_ids"))
	tz, err := normalizeTimezone(query.Get("timezone"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid timezone")
		return
	}

	rows, err := h.Analytics.Timeseries(r.Context(), family.ID, analyticsdomain.TimeseriesFilter{
		From:        from,
		To:          to,
		GroupBy:     groupBy,
		Currency:    currency,
		CategoryIDs: categoryIDs,
		Timezone:    tz,
	})
	if err != nil {
		h.log.InternalError("analytics.timeseries: build timeseries failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) AnalyticsByCategory(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("analytics.by_category: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("analytics.by_category: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	from, err := parseDateRequired(query.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from is required")
		return
	}
	to, err := parseDateRequired(query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "to is required")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "invalid_request", "from must be <= to")
		return
	}

	limit, err := parseIntParam(query.Get("limit"), 20)
	if err != nil || limit <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
		return
	}

	currency := strings.TrimSpace(query.Get("currency"))
	categoryIDs := parseCSV(query.Get("category_ids"))

	rows, err := h.Analytics.ByCategory(r.Context(), family.ID, analyticsdomain.ByCategoryFilter{
		From:        from,
		To:          to,
		Currency:    currency,
		CategoryIDs: categoryIDs,
		Limit:       limit,
	})
	if err != nil {
		h.log.InternalError("analytics.by_category: build report failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) TopCategories(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("analytics.top_categories: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("analytics.top_categories: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	result, err := h.Analytics.TopCategories(r.Context(), family.ID)
	if err != nil {
		h.log.InternalError("analytics.top_categories: build report failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handlers) ReportsMonthly(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("reports.monthly: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("reports.monthly: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	fromMonth, err := parseMonthRequired(query.Get("from_month"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from_month is required")
		return
	}
	toMonth, err := parseMonthRequired(query.Get("to_month"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "to_month is required")
		return
	}
	if toMonth.Before(fromMonth) {
		writeError(w, http.StatusBadRequest, "invalid_request", "from_month must be <= to_month")
		return
	}

	from := time.Date(fromMonth.Year(), fromMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	toExclusive := time.Date(toMonth.Year(), toMonth.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)

	currency := strings.TrimSpace(query.Get("currency"))
	categoryIDs := parseCSV(query.Get("category_ids"))

	rows, err := h.Analytics.Monthly(r.Context(), family.ID, analyticsdomain.MonthlyFilter{
		From:        from,
		To:          toExclusive,
		Currency:    currency,
		CategoryIDs: categoryIDs,
	})
	if err != nil {
		h.log.InternalError("reports.monthly: build report failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) ReportsCompare(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("reports.compare: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("reports.compare: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	fromA, err := parseDateRequired(query.Get("from_a"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from_a is required")
		return
	}
	toA, err := parseDateRequired(query.Get("to_a"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "to_a is required")
		return
	}
	fromB, err := parseDateRequired(query.Get("from_b"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "from_b is required")
		return
	}
	toB, err := parseDateRequired(query.Get("to_b"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "to_b is required")
		return
	}

	if toA.Before(fromA) || toB.Before(fromB) {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid period range")
		return
	}

	currency := strings.TrimSpace(query.Get("currency"))
	categoryIDs := parseCSV(query.Get("category_ids"))

	result, err := h.Analytics.Compare(r.Context(), family.ID, analyticsdomain.CompareFilter{
		FromA:       fromA,
		ToA:         toA,
		FromB:       fromB,
		ToB:         toB,
		Currency:    currency,
		CategoryIDs: categoryIDs,
	})
	if err != nil {
		h.log.InternalError("reports.compare: build report failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func normalizeTimezone(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Europe/Moscow", nil
	}
	_, err := time.LoadLocation(value)
	if err != nil {
		return "", err
	}
	return value, nil
}
