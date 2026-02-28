package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	familydomain "family-app-go/internal/domain/family"
	syncdomain "family-app-go/internal/domain/sync"
	"family-app-go/internal/transport/httpserver/middleware"
)

const (
	minIdempotencyKeyLength = 8
	maxIdempotencyKeyLength = 128
)

var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

type syncBatchRequest struct {
	Operations []syncOperationRequest `json:"operations"`
}

type syncOperationRequest struct {
	OperationID string          `json:"operation_id"`
	Type        string          `json:"type"`
	LocalID     string          `json:"local_id"`
	Payload     json.RawMessage `json:"payload"`
}

type syncCreateTodoPayloadRequest struct {
	ListID string `json:"list_id"`
	Title  string `json:"title"`
}

type syncSetTodoCompletedPayloadRequest struct {
	TodoID      *string `json:"todo_id"`
	TodoLocalID *string `json:"todo_local_id"`
	IsCompleted *bool   `json:"is_completed"`
}

func (h *Handlers) SyncBatch(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()

	var req syncBatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid json body")
		return
	}

	if len(req.Operations) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "operations are required")
		return
	}
	if len(req.Operations) > syncdomain.MaxBatchOperations {
		writeError(w, http.StatusRequestEntityTooLarge, "sync_batch_too_large", "too many operations in one batch")
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey != "" && len(idempotencyKey) < minIdempotencyKeyLength {
		writeError(w, http.StatusBadRequest, "invalid_request", "idempotency key is too short")
		return
	}
	if len(idempotencyKey) > maxIdempotencyKeyLength {
		writeError(w, http.StatusBadRequest, "invalid_request", "idempotency key is too long")
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
			h.log.BusinessError("sync.batch: family not found", err, "user_id", user.ID)
			writeError(w, http.StatusNotFound, "family_not_found", "family not found")
			return
		}
		h.log.InternalError("sync.batch: get family failed", err, "user_id", user.ID)
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	operations := make([]syncdomain.OperationInput, 0, len(req.Operations))
	for i, operation := range req.Operations {
		parsed, err := parseSyncOperation(operation)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid operation at index "+strconv.Itoa(i))
			return
		}
		operations = append(operations, parsed)
	}

	response, err := h.Sync.ProcessBatch(r.Context(), syncdomain.BatchInput{
		FamilyID:       family.ID,
		User:           syncdomain.UserSnapshot{ID: user.ID, Name: user.Name, Email: user.Email, AvatarURL: user.AvatarURL},
		IdempotencyKey: idempotencyKey,
		Operations:     operations,
	})
	if err != nil {
		logAttrs := []any{
			"user_id", user.ID,
			"family_id", family.ID,
			"operations", len(operations),
			"has_idempotency_key", idempotencyKey != "",
			"duration_ms", time.Since(startedAt).Milliseconds(),
		}

		switch {
		case errors.Is(err, syncdomain.ErrBatchTooLarge):
			h.log.BusinessError("sync.batch: batch too large", err, logAttrs...)
			writeError(w, http.StatusRequestEntityTooLarge, "sync_batch_too_large", "too many operations in one batch")
		case errors.Is(err, syncdomain.ErrIdempotencyKeyPayloadMismatch):
			h.log.BusinessError("sync.batch: idempotency key payload mismatch", err, logAttrs...)
			writeError(w, http.StatusConflict, "idempotency_key_payload_mismatch", "Idempotency-Key was already used with different payload")
		case errors.Is(err, syncdomain.ErrBatchInProgress):
			h.log.BusinessError("sync.batch: batch in progress", err, logAttrs...)
			writeError(w, http.StatusConflict, "batch_in_progress", "sync batch is already in progress")
		default:
			h.log.InternalError("sync.batch: process batch failed", err, logAttrs...)
			writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	h.log.Info(
		"sync: completed",
		"sync_id",
		response.SyncID,
		"user_id",
		user.ID,
		"family_id",
		family.ID,
		"status",
		response.Status,
		"total",
		response.Summary.Total,
		"applied",
		response.Summary.Applied,
		"duplicate",
		response.Summary.Duplicate,
		"failed",
		response.Summary.Failed,
		"has_idempotency_key",
		idempotencyKey != "",
		"duration_ms",
		time.Since(startedAt).Milliseconds(),
	)

	writeJSON(w, http.StatusOK, response)
}

func parseSyncOperation(operation syncOperationRequest) (syncdomain.OperationInput, error) {
	operationID := strings.TrimSpace(operation.OperationID)
	if !isUUID(operationID) {
		return syncdomain.OperationInput{}, errors.New("invalid operation_id")
	}

	operationType := syncdomain.OperationType(strings.TrimSpace(operation.Type))
	localID := strings.TrimSpace(operation.LocalID)

	result := syncdomain.OperationInput{
		OperationID: operationID,
		Type:        operationType,
		LocalID:     localID,
	}

	switch operationType {
	case syncdomain.OperationTypeCreateExpense:
		if localID == "" {
			return syncdomain.OperationInput{}, errors.New("local_id is required")
		}

		var payload createExpenseRequest
		if err := decodePayload(operation.Payload, &payload); err != nil {
			return syncdomain.OperationInput{}, err
		}

		date, err := parseDateRequired(payload.Date)
		if err != nil {
			return syncdomain.OperationInput{}, err
		}
		if payload.Amount <= 0 {
			return syncdomain.OperationInput{}, errors.New("amount must be positive")
		}
		if strings.TrimSpace(payload.Currency) == "" {
			return syncdomain.OperationInput{}, errors.New("currency is required")
		}
		if strings.TrimSpace(payload.Title) == "" {
			return syncdomain.OperationInput{}, errors.New("title is required")
		}

		result.CreateExpense = &syncdomain.CreateExpensePayload{
			Date:        date,
			Amount:      payload.Amount,
			Currency:    payload.Currency,
			Title:       payload.Title,
			CategoryIDs: payload.CategoryIDs,
		}
		return result, nil

	case syncdomain.OperationTypeCreateTodo:
		if localID == "" {
			return syncdomain.OperationInput{}, errors.New("local_id is required")
		}

		var payload syncCreateTodoPayloadRequest
		if err := decodePayload(operation.Payload, &payload); err != nil {
			return syncdomain.OperationInput{}, err
		}
		if strings.TrimSpace(payload.ListID) == "" {
			return syncdomain.OperationInput{}, errors.New("list_id is required")
		}
		if strings.TrimSpace(payload.Title) == "" {
			return syncdomain.OperationInput{}, errors.New("title is required")
		}

		result.CreateTodo = &syncdomain.CreateTodoPayload{
			ListID: payload.ListID,
			Title:  payload.Title,
		}
		return result, nil

	case syncdomain.OperationTypeSetTodoCompleted:
		var payload syncSetTodoCompletedPayloadRequest
		if err := decodePayload(operation.Payload, &payload); err != nil {
			return syncdomain.OperationInput{}, err
		}
		if payload.IsCompleted == nil {
			return syncdomain.OperationInput{}, errors.New("is_completed is required")
		}

		todoID := normalizeStringPtr(payload.TodoID)
		todoLocalID := normalizeStringPtr(payload.TodoLocalID)
		if todoID == nil && todoLocalID == nil {
			return syncdomain.OperationInput{}, errors.New("todo_id or todo_local_id is required")
		}

		result.SetTodoCompleted = &syncdomain.SetTodoCompletedPayload{
			TodoID:      valueOrEmptyPtr(todoID),
			TodoLocalID: valueOrEmptyPtr(todoLocalID),
			IsCompleted: *payload.IsCompleted,
		}
		return result, nil

	default:
		return result, nil
	}
}

func decodePayload(raw json.RawMessage, dst interface{}) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid payload")
	}
	return nil
}

func isUUID(value string) bool {
	return uuidRegex.MatchString(strings.TrimSpace(value))
}

func normalizeStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func valueOrEmptyPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
