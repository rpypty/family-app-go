package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

type createExpenseRequest struct {
	Date     string   `json:"date"`
	Amount   float64  `json:"amount"`
	Currency string   `json:"currency"`
	Title    string   `json:"title"`
	TagIDs   []string `json:"tag_ids"`
}

type updateExpenseRequest struct {
	Date     string   `json:"date"`
	Amount   float64  `json:"amount"`
	Currency string   `json:"currency"`
	Title    string   `json:"title"`
	TagIDs   []string `json:"tag_ids"`
}

func (h *Handlers) ListExpenses(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("expenses.list: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("expenses.list: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	query := r.URL.Query()
	from, err := parseDateParam(query.Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid from date")
		return
	}
	to, err := parseDateParam(query.Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid to date")
		return
	}

	limit, err := parseIntParam(query.Get("limit"), 50)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid limit")
		return
	}
	offset, err := parseIntParam(query.Get("offset"), 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid offset")
		return
	}

	filter := expensesdomain.ListFilter{
		From:   from,
		To:     to,
		Limit:  limit,
		Offset: offset,
	}
	tagIDs := parseCSV(query.Get("tag_ids"))
	if len(tagIDs) > 0 {
		filter.TagIDs = tagIDs
	} else {
		tagID := strings.TrimSpace(query.Get("tag_id"))
		if tagID != "" {
			filter.TagIDs = []string{tagID}
		}
	}

	items, total, err := h.Expenses.ListExpenses(r.Context(), family.ID, filter)
	if err != nil {
		h.log.InternalError("expenses.list: list expenses failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	response := make([]expenseResponse, 0, len(items))
	for _, expense := range items {
		response = append(response, toExpenseResponse(expense))
	}

	writeJSON(w, http.StatusOK, expenseListResponse{
		Items: response,
		Total: total,
	})
}

func (h *Handlers) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var req createExpenseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("expenses.create: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("expenses.create: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	date, err := parseDateRequired(req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "amount must be positive")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	if strings.TrimSpace(req.Currency) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "currency is required")
		return
	}

	input := expensesdomain.CreateExpenseInput{
		FamilyID: family.ID,
		UserID:   user.ID,
		Date:     date,
		Amount:   req.Amount,
		Currency: req.Currency,
		Title:    req.Title,
		TagIDs:   req.TagIDs,
	}

	created, err := h.Expenses.CreateExpense(r.Context(), input)
	if err != nil {
		if errors.Is(err, expensesdomain.ErrTagNotFound) {
			h.log.BusinessError("expenses.create: tag not found", err, "user_id", user.ID, "family_id", family.ID)
			writeError(w, http.StatusNotFound, "tag_not_found", "tag not found")
			return
		}
		h.log.InternalError("expenses.create: create expense failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toExpenseResponse(*created))
}

func (h *Handlers) UpdateExpense(w http.ResponseWriter, r *http.Request) {
	var req updateExpenseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	expenseID := strings.TrimSpace(chi.URLParam(r, "id"))
	if expenseID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("expenses.update: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("expenses.update: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	date, err := parseDateRequired(req.Date)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
		return
	}
	if req.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "amount must be positive")
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}
	if strings.TrimSpace(req.Currency) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "currency is required")
		return
	}

	input := expensesdomain.UpdateExpenseInput{
		ID:       expenseID,
		FamilyID: family.ID,
		Date:     date,
		Amount:   req.Amount,
		Currency: req.Currency,
		Title:    req.Title,
		TagIDs:   req.TagIDs,
	}

	updated, err := h.Expenses.UpdateExpense(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, expensesdomain.ErrExpenseNotFound):
			h.log.BusinessError("expenses.update: expense not found", err, "user_id", user.ID, "family_id", family.ID, "expense_id", expenseID)
			writeError(w, http.StatusNotFound, "expense_not_found", "expense not found")
		case errors.Is(err, expensesdomain.ErrTagNotFound):
			h.log.BusinessError("expenses.update: tag not found", err, "user_id", user.ID, "family_id", family.ID, "expense_id", expenseID)
			writeError(w, http.StatusNotFound, "tag_not_found", "tag not found")
		default:
			h.log.InternalError("expenses.update: update expense failed", err, "user_id", user.ID, "family_id", family.ID, "expense_id", expenseID)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	writeJSON(w, http.StatusOK, toExpenseResponse(*updated))
}

func (h *Handlers) DeleteExpense(w http.ResponseWriter, r *http.Request) {
	expenseID := strings.TrimSpace(chi.URLParam(r, "id"))
	if expenseID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError("expenses.delete: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("expenses.delete: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	if err := h.Expenses.DeleteExpense(r.Context(), family.ID, expenseID); err != nil {
		if errors.Is(err, expensesdomain.ErrExpenseNotFound) {
			h.log.BusinessError("expenses.delete: expense not found", err, "user_id", user.ID, "family_id", family.ID, "expense_id", expenseID)
			writeError(w, http.StatusNotFound, "expense_not_found", "expense not found")
			return
		}
		h.log.InternalError("expenses.delete: delete expense failed", err, "user_id", user.ID, "family_id", family.ID, "expense_id", expenseID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

func toExpenseResponse(expense expensesdomain.ExpenseWithTags) expenseResponse {
	return expenseResponse{
		ID:        expense.ID,
		FamilyID:  expense.FamilyID,
		UserID:    expense.UserID,
		Date:      expense.Date.Format("2006-01-02"),
		Amount:    expense.Amount,
		Currency:  expense.Currency,
		Title:     expense.Title,
		TagIDs:    expense.TagIDs,
		CreatedAt: expense.CreatedAt,
		UpdatedAt: expense.UpdatedAt,
	}
}
