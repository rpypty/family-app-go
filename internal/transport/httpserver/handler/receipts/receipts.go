package receipts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	receiptsdomain "family-app-go/internal/domain/receipts"
	"family-app-go/internal/transport/httpserver/middleware"
	"github.com/go-chi/chi/v5"
)

const maxReceiptFileSizeBytes = 8 * 1024 * 1024

type activeParseResponse struct {
	Item *receiptParseSummaryResponse `json:"item"`
}

type receiptParseSummaryResponse struct {
	ID        string                     `json:"id"`
	Status    receiptsdomain.ParseStatus `json:"status"`
	CreatedAt time.Time                  `json:"created_at"`
	UpdatedAt time.Time                  `json:"updated_at"`
}

type receiptParseResponse struct {
	ID              string                        `json:"id"`
	Status          receiptsdomain.ParseStatus    `json:"status"`
	CreatedAt       time.Time                     `json:"created_at"`
	UpdatedAt       time.Time                     `json:"updated_at"`
	Receipt         receiptMetaResponse           `json:"receipt"`
	DraftExpenses   []receiptDraftExpenseResponse `json:"draft_expenses"`
	Items           []receiptItemResponse         `json:"items"`
	UnresolvedItems []receiptItemResponse         `json:"unresolved_items"`
	Warnings        []string                      `json:"warnings"`
	Error           *receiptParseErrorResponse    `json:"error,omitempty"`
}

type receiptMetaResponse struct {
	MerchantName  *string  `json:"merchant_name"`
	PurchasedAt   *string  `json:"purchased_at"`
	RequestedDate *string  `json:"requested_date"`
	Currency      *string  `json:"currency"`
	DetectedTotal *float64 `json:"detected_total"`
	ItemsTotal    *float64 `json:"items_total"`
}

type receiptDraftExpenseResponse struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Amount     float64  `json:"amount"`
	Currency   string   `json:"currency"`
	CategoryID string   `json:"category_id"`
	Confidence *float64 `json:"confidence"`
	Warnings   []string `json:"warnings"`
}

type receiptItemResponse struct {
	ID                    string   `json:"id"`
	RawName               string   `json:"raw_name"`
	NormalizedName        *string  `json:"normalized_name"`
	Quantity              *float64 `json:"quantity"`
	UnitPrice             *float64 `json:"unit_price"`
	LineTotal             float64  `json:"line_total"`
	EffectiveLineTotal    *float64 `json:"effective_line_total"`
	LLMCategoryID         *string  `json:"llm_category_id"`
	LLMCategoryConfidence *float64 `json:"llm_category_confidence"`
	FinalCategoryID       *string  `json:"final_category_id"`
	EditedByUser          bool     `json:"edited_by_user"`
}

type receiptParseErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type approveParseRequest struct {
	Expenses []approveExpenseRequest `json:"expenses"`
}

type updateItemsRequest struct {
	Items []updateItemRequest `json:"items"`
}

type updateItemRequest struct {
	ID         string   `json:"id"`
	Amount     *float64 `json:"amount"`
	CategoryID *string  `json:"category_id"`
}

type approveExpenseRequest struct {
	DraftID     string   `json:"draft_id"`
	Title       string   `json:"title"`
	Amount      float64  `json:"amount"`
	Currency    string   `json:"currency"`
	CategoryIDs []string `json:"category_ids"`
	Date        string   `json:"date"`
}

type approveParseResponse struct {
	Status   receiptsdomain.ParseStatus `json:"status"`
	Expenses []expenseResponse          `json:"expenses"`
}

type expenseResponse struct {
	ID           string    `json:"id"`
	FamilyID     string    `json:"family_id"`
	UserID       string    `json:"user_id"`
	Date         string    `json:"date"`
	Amount       float64   `json:"amount"`
	Currency     string    `json:"currency"`
	BaseCurrency *string   `json:"base_currency,omitempty"`
	ExchangeRate *float64  `json:"exchange_rate,omitempty"`
	AmountInBase *float64  `json:"amount_in_base,omitempty"`
	RateDate     *string   `json:"rate_date,omitempty"`
	RateSource   *string   `json:"rate_source,omitempty"`
	Title        string    `json:"title"`
	CategoryIDs  []string  `json:"category_ids"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (h *Handlers) CreateParse(w http.ResponseWriter, r *http.Request) {
	user, family, ok := h.currentUserFamily(w, r, "receipt_parses.create")
	if !ok {
		return
	}

	input, err := parseCreateParseForm(w, r, family.ID, user.ID, family.DefaultCurrency)
	if err != nil {
		writeReceiptError(w, err)
		return
	}

	job, err := h.Receipts.CreateParse(r.Context(), input)
	if err != nil {
		h.writeServiceError(w, err, "receipt_parses.create", user.ID, family.ID, "")
		return
	}

	writeJSON(w, http.StatusAccepted, receiptParseSummaryResponse{
		ID:        job.ID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	})
}

func (h *Handlers) GetActiveParse(w http.ResponseWriter, r *http.Request) {
	user, family, ok := h.currentUserFamily(w, r, "receipt_parses.active")
	if !ok {
		return
	}

	job, err := h.Receipts.GetActiveParse(r.Context(), family.ID)
	if err != nil {
		h.log.InternalError("receipt_parses.active: get active parse failed", err, "user_id", user.ID, "family_id", family.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}
	if job == nil {
		writeJSON(w, http.StatusOK, activeParseResponse{Item: nil})
		return
	}

	writeJSON(w, http.StatusOK, activeParseResponse{
		Item: &receiptParseSummaryResponse{
			ID:        job.ID,
			Status:    job.Status,
			CreatedAt: job.CreatedAt,
			UpdatedAt: job.UpdatedAt,
		},
	})
}

func (h *Handlers) GetParse(w http.ResponseWriter, r *http.Request) {
	user, family, ok := h.currentUserFamily(w, r, "receipt_parses.get")
	if !ok {
		return
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "id"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	job, err := h.Receipts.GetParse(r.Context(), family.ID, jobID)
	if err != nil {
		h.writeServiceError(w, err, "receipt_parses.get", user.ID, family.ID, jobID)
		return
	}

	writeJSON(w, http.StatusOK, toReceiptParseResponse(*job))
}

func (h *Handlers) ApproveParse(w http.ResponseWriter, r *http.Request) {
	var req approveParseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	user, family, ok := h.currentUserFamily(w, r, "receipt_parses.approve")
	if !ok {
		return
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "id"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	inputs := make([]receiptsdomain.ApproveExpenseInput, 0, len(req.Expenses))
	for _, item := range req.Expenses {
		date, err := parseDateRequired(item.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid date")
			return
		}
		inputs = append(inputs, receiptsdomain.ApproveExpenseInput{
			DraftID:     strings.TrimSpace(item.DraftID),
			Date:        date,
			Title:       item.Title,
			Amount:      item.Amount,
			Currency:    item.Currency,
			CategoryIDs: item.CategoryIDs,
		})
	}

	created, err := h.Receipts.ApproveParse(r.Context(), receiptsdomain.ApproveInput{
		FamilyID:     family.ID,
		UserID:       user.ID,
		BaseCurrency: family.DefaultCurrency,
		JobID:        jobID,
		Expenses:     inputs,
	})
	if err != nil {
		h.writeServiceError(w, err, "receipt_parses.approve", user.ID, family.ID, jobID)
		return
	}

	expenses := make([]expenseResponse, 0, len(created))
	for _, expense := range created {
		expenses = append(expenses, toExpenseResponse(expense))
	}
	writeJSON(w, http.StatusOK, approveParseResponse{
		Status:   receiptsdomain.StatusApproved,
		Expenses: expenses,
	})
}

func (h *Handlers) UpdateItems(w http.ResponseWriter, r *http.Request) {
	var req updateItemsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	user, family, ok := h.currentUserFamily(w, r, "receipt_parses.update_items")
	if !ok {
		return
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "id"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	items := make([]receiptsdomain.ReviewItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, receiptsdomain.ReviewItemInput{
			ItemID:     strings.TrimSpace(item.ID),
			Amount:     item.Amount,
			CategoryID: item.CategoryID,
		})
	}

	job, err := h.Receipts.UpdateItems(r.Context(), receiptsdomain.UpdateItemsInput{
		FamilyID: family.ID,
		JobID:    jobID,
		Items:    items,
	})
	if err != nil {
		h.writeServiceError(w, err, "receipt_parses.update_items", user.ID, family.ID, jobID)
		return
	}

	writeJSON(w, http.StatusOK, toReceiptParseResponse(*job))
}

func (h *Handlers) CancelParse(w http.ResponseWriter, r *http.Request) {
	user, family, ok := h.currentUserFamily(w, r, "receipt_parses.cancel")
	if !ok {
		return
	}
	jobID := strings.TrimSpace(chi.URLParam(r, "id"))
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "id is required")
		return
	}

	job, err := h.Receipts.CancelParse(r.Context(), family.ID, jobID)
	if err != nil {
		h.writeServiceError(w, err, "receipt_parses.cancel", user.ID, family.ID, jobID)
		return
	}

	writeJSON(w, http.StatusOK, receiptParseSummaryResponse{
		ID:        job.ID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
	})
}

func (h *Handlers) currentUserFamily(w http.ResponseWriter, r *http.Request, operation string) (middleware.User, *familydomain.Family, bool) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_token", "invalid token")
		return middleware.User{}, nil, false
	}

	family, err := h.Families.GetFamilyByUser(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, familydomain.ErrFamilyNotFound) {
			h.log.BusinessError(operation+": family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return middleware.User{}, nil, false
		}
		h.log.InternalError(operation+": get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return middleware.User{}, nil, false
	}

	return user, family, true
}

func (h *Handlers) writeServiceError(w http.ResponseWriter, err error, operation, userID, familyID, jobID string) {
	switch {
	case errors.Is(err, receiptsdomain.ErrReceiptParserDisabled):
		h.log.BusinessError(operation+": parser disabled", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusServiceUnavailable, "receipt_parser_disabled", "receipt parser is disabled")
	case errors.Is(err, receiptsdomain.ErrActiveReceiptParseExists):
		h.log.BusinessError(operation+": active parse exists", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusConflict, "active_receipt_parse_exists", "active receipt parse already exists")
	case errors.Is(err, receiptsdomain.ErrReceiptParseNotFound):
		h.log.BusinessError(operation+": parse not found", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusNotFound, "receipt_parse_not_found", "receipt parse not found")
	case errors.Is(err, receiptsdomain.ErrReceiptParseInvalidStatus):
		h.log.BusinessError(operation+": invalid status", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusConflict, "receipt_parse_invalid_status", "receipt parse has invalid status")
	case errors.Is(err, receiptsdomain.ErrInvalidReceiptFile):
		h.log.BusinessError(operation+": invalid file", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusBadRequest, "invalid_receipt_file", "invalid receipt file")
	case errors.Is(err, receiptsdomain.ErrReceiptFileTooLarge):
		h.log.BusinessError(operation+": file too large", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusRequestEntityTooLarge, "receipt_file_too_large", "receipt file is too large")
	case errors.Is(err, receiptsdomain.ErrCategorySelectionRequired):
		h.log.BusinessError(operation+": category selection required", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusBadRequest, "category_selection_required", "category selection is required")
	case errors.Is(err, receiptsdomain.ErrCategoryNotFound):
		h.log.BusinessError(operation+": category not found", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusNotFound, "category_not_found", "category not found")
	case errors.Is(err, receiptsdomain.ErrReceiptParseEmpty):
		h.log.BusinessError(operation+": parse empty", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusUnprocessableEntity, "receipt_parse_empty", "receipt parse produced no draft expenses")
	case errors.Is(err, receiptsdomain.ErrReceiptParseUnresolvedItems):
		h.log.BusinessError(operation+": unresolved items", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusConflict, "receipt_parse_unresolved_items", "receipt parse has unresolved items")
	case errors.Is(err, expensesdomain.ErrRateNotAvailable):
		h.log.BusinessError(operation+": rate not available", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusUnprocessableEntity, "rate_not_available", "rate is not available for selected date")
	default:
		h.log.InternalError(operation+": request failed", err, "user_id", userID, "family_id", familyID, "job_id", jobID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func parseCreateParseForm(w http.ResponseWriter, r *http.Request, familyID, userID, defaultCurrency string) (receiptsdomain.CreateParseInput, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxReceiptFileSizeBytes+1024*1024)
	if err := r.ParseMultipartForm(maxReceiptFileSizeBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return receiptsdomain.CreateParseInput{}, receiptsdomain.ErrReceiptFileTooLarge
		}
		return receiptsdomain.CreateParseInput{}, receiptsdomain.ErrInvalidReceiptFile
	}

	file, header, err := r.FormFile("receipt")
	if err != nil {
		return receiptsdomain.CreateParseInput{}, receiptsdomain.ErrInvalidReceiptFile
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxReceiptFileSizeBytes+1))
	if err != nil {
		return receiptsdomain.CreateParseInput{}, receiptsdomain.ErrInvalidReceiptFile
	}
	if len(data) > maxReceiptFileSizeBytes {
		return receiptsdomain.CreateParseInput{}, receiptsdomain.ErrReceiptFileTooLarge
	}

	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = mediaType
	}
	hash := sha256.Sum256(data)

	allCategories := parseBoolish(r.FormValue("all_categories"))
	categoryIDs := formValuesCSV(r, "category_ids")
	mode := receiptsdomain.CategoryModeSelected
	if allCategories {
		mode = receiptsdomain.CategoryModeAll
	}

	requestedDate, err := parseDateParam(r.FormValue("date"))
	if err != nil {
		return receiptsdomain.CreateParseInput{}, err
	}
	currency := strings.ToUpper(strings.TrimSpace(r.FormValue("currency")))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(defaultCurrency))
	}

	return receiptsdomain.CreateParseInput{
		FamilyID:            familyID,
		UserID:              userID,
		CategoryMode:        mode,
		SelectedCategoryIDs: categoryIDs,
		RequestedDate:       requestedDate,
		RequestedCurrency:   currency,
		File: receiptsdomain.UploadedFile{
			FileName:    header.Filename,
			ContentType: contentType,
			SizeBytes:   int64(len(data)),
			SHA256:      hex.EncodeToString(hash[:]),
			Data:        data,
		},
	}, nil
}

func writeReceiptError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, receiptsdomain.ErrReceiptFileTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, "receipt_file_too_large", "receipt file is too large")
	case errors.Is(err, receiptsdomain.ErrInvalidReceiptFile):
		writeError(w, http.StatusBadRequest, "invalid_receipt_file", "invalid receipt file")
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request")
	}
}

func toReceiptParseResponse(job receiptsdomain.JobWithDrafts) receiptParseResponse {
	var purchasedAt *string
	if job.PurchasedAt != nil {
		value := job.PurchasedAt.Format("2006-01-02")
		purchasedAt = &value
	}
	var requestedDate *string
	if job.RequestedDate != nil {
		value := job.RequestedDate.Format("2006-01-02")
		requestedDate = &value
	}

	drafts := make([]receiptDraftExpenseResponse, 0, len(job.DraftExpenses))
	items := make([]receiptItemResponse, 0, len(job.Items))
	unresolvedItems := make([]receiptItemResponse, 0)
	warnings := []string{}
	for _, draft := range job.DraftExpenses {
		draftWarnings := decodeWarnings(draft.Warnings)
		warnings = append(warnings, draftWarnings...)
		drafts = append(drafts, receiptDraftExpenseResponse{
			ID:         draft.ID,
			Title:      draft.Title,
			Amount:     draft.Amount,
			Currency:   draft.Currency,
			CategoryID: draft.CategoryID,
			Confidence: draft.Confidence,
			Warnings:   draftWarnings,
		})
	}

	for _, item := range job.Items {
		responseItem := receiptItemResponse{
			ID:                    item.ID,
			RawName:               item.RawName,
			NormalizedName:        item.NormalizedName,
			Quantity:              item.Quantity,
			UnitPrice:             item.UnitPrice,
			LineTotal:             item.LineTotal,
			EffectiveLineTotal:    item.FinalLineTotal,
			LLMCategoryID:         item.LLMCategoryID,
			LLMCategoryConfidence: item.LLMCategoryConfidence,
			FinalCategoryID:       item.FinalCategoryID,
			EditedByUser:          item.EditedByUser,
		}
		if item.FinalCategoryID == nil || strings.TrimSpace(*item.FinalCategoryID) == "" {
			unresolvedItems = append(unresolvedItems, responseItem)
			continue
		}
		items = append(items, responseItem)
	}

	var parseError *receiptParseErrorResponse
	if job.ErrorCode != nil || job.ErrorMessage != nil {
		parseError = &receiptParseErrorResponse{
			Code:    stringValue(job.ErrorCode),
			Message: stringValue(job.ErrorMessage),
		}
	}

	return receiptParseResponse{
		ID:        job.ID,
		Status:    job.Status,
		CreatedAt: job.CreatedAt,
		UpdatedAt: job.UpdatedAt,
		Receipt: receiptMetaResponse{
			MerchantName:  job.MerchantName,
			PurchasedAt:   purchasedAt,
			RequestedDate: requestedDate,
			Currency:      job.Currency,
			DetectedTotal: job.DetectedTotal,
			ItemsTotal:    job.ItemsTotal,
		},
		DraftExpenses:   drafts,
		Items:           items,
		UnresolvedItems: unresolvedItems,
		Warnings:        warnings,
		Error:           parseError,
	}
}

func toExpenseResponse(expense expensesdomain.ExpenseWithCategories) expenseResponse {
	var rateDate *string
	if expense.RateDate != nil {
		value := expense.RateDate.Format("2006-01-02")
		rateDate = &value
	}

	return expenseResponse{
		ID:           expense.ID,
		FamilyID:     expense.FamilyID,
		UserID:       expense.UserID,
		Date:         expense.Date.Format("2006-01-02"),
		Amount:       expense.Amount,
		Currency:     expense.Currency,
		BaseCurrency: expense.BaseCurrency,
		ExchangeRate: expense.ExchangeRate,
		AmountInBase: expense.AmountInBase,
		RateDate:     rateDate,
		RateSource:   expense.RateSource,
		Title:        expense.Title,
		CategoryIDs:  expense.CategoryIDs,
		CreatedAt:    expense.CreatedAt,
		UpdatedAt:    expense.UpdatedAt,
	}
}

func formValuesCSV(r *http.Request, key string) []string {
	if r.MultipartForm == nil {
		return nil
	}
	values := r.MultipartForm.Value[key]
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, parseCSV(value)...)
	}
	return result
}

func parseBoolish(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func decodeWarnings(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var warnings []string
	if err := json.Unmarshal(raw, &warnings); err != nil {
		return []string{}
	}
	return warnings
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
