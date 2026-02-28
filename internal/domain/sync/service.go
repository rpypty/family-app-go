package sync

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	todosdomain "family-app-go/internal/domain/todos"
)

type ExpensesService interface {
	CreateExpense(ctx context.Context, input expensesdomain.CreateExpenseInput) (*expensesdomain.ExpenseWithCategories, error)
}

type TodosService interface {
	CreateTodoItem(ctx context.Context, familyID string, input todosdomain.CreateTodoItemInput) (*todosdomain.TodoItem, error)
	UpdateTodoItem(ctx context.Context, input todosdomain.UpdateTodoItemInput) (*todosdomain.TodoItem, error)
}

type Service struct {
	repo     Repository
	expenses ExpensesService
	todos    TodosService
}

func NewService(repo Repository, expenses ExpensesService, todos TodosService) *Service {
	return &Service{
		repo:     repo,
		expenses: expenses,
		todos:    todos,
	}
}

func (s *Service) ProcessBatch(ctx context.Context, input BatchInput) (*BatchResponse, error) {
	if len(input.Operations) == 0 {
		return nil, fmt.Errorf("operations are required")
	}
	if len(input.Operations) > MaxBatchOperations {
		return nil, ErrBatchTooLarge
	}

	syncID, err := newUUID()
	if err != nil {
		return nil, err
	}

	requestHash, err := hashRequest(input.Operations)
	if err != nil {
		return nil, err
	}

	idempotencyKey := strings.TrimSpace(input.IdempotencyKey)
	batchCreated := false

	if idempotencyKey != "" {
		batch := &BatchRecord{
			ID:             syncID,
			FamilyID:       input.FamilyID,
			UserID:         input.User.ID,
			IdempotencyKey: &idempotencyKey,
			RequestHash:    requestHash,
			Status:         BatchStateProcessing,
		}

		created, existing, err := s.repo.BeginBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		if !created {
			if existing == nil {
				return nil, ErrBatchInProgress
			}
			if existing.RequestHash != requestHash {
				return nil, ErrIdempotencyKeyPayloadMismatch
			}
			if existing.Status == BatchStateCompleted && len(existing.ResponseJSON) > 0 {
				var cached BatchResponse
				if err := json.Unmarshal(existing.ResponseJSON, &cached); err == nil {
					return &cached, nil
				}
			}
			return nil, ErrBatchInProgress
		}

		batchCreated = true
	}

	response := BatchResponse{
		SyncID:   syncID,
		Results:  make([]OperationResult, 0, len(input.Operations)),
		Mappings: make([]EntityMapping, 0),
		Summary: BatchSummary{
			Total: len(input.Operations),
		},
		ServerTime: time.Now().UTC(),
	}

	localTodoIDs := make(map[string]string)

	for _, operation := range input.Operations {
		result, mapping := s.processOperation(ctx, input, operation, localTodoIDs)
		response.Results = append(response.Results, result)
		if mapping != nil {
			response.Mappings = append(response.Mappings, *mapping)
			if mapping.Entity == EntityTodoItem {
				localTodoIDs[mapping.LocalID] = mapping.ServerID
			}
		}

		switch result.Status {
		case ResultStatusApplied:
			response.Summary.Applied++
		case ResultStatusDuplicate:
			response.Summary.Duplicate++
		default:
			response.Summary.Failed++
		}
	}

	response.Status = deriveBatchStatus(response.Summary)

	if batchCreated {
		if encoded, err := json.Marshal(response); err == nil {
			_ = s.repo.CompleteBatch(ctx, syncID, BatchStateCompleted, encoded)
		}
	}

	return &response, nil
}

func (s *Service) processOperation(ctx context.Context, input BatchInput, operation OperationInput, localTodoIDs map[string]string) (OperationResult, *EntityMapping) {
	base := OperationResult{
		OperationID: operation.OperationID,
		Type:        operation.Type,
	}

	payloadHash, err := hashOperation(operation)
	if err != nil {
		return failResult(base, ErrorCodeInternalError, "internal error", true), nil
	}

	recordID, err := newUUID()
	if err != nil {
		return failResult(base, ErrorCodeInternalError, "internal error", true), nil
	}

	reserved := &OperationRecord{
		ID:            recordID,
		FamilyID:      input.FamilyID,
		UserID:        input.User.ID,
		OperationID:   operation.OperationID,
		OperationType: operation.Type,
		PayloadHash:   payloadHash,
		Status:        OperationStatePending,
	}
	if operation.LocalID != "" {
		localID := operation.LocalID
		reserved.LocalID = &localID
	}

	created, existing, err := s.repo.ReserveOperation(ctx, reserved)
	if err != nil {
		return failResult(base, ErrorCodeInternalError, "internal error", true), nil
	}
	if !created {
		return resultFromExisting(base, operation, existing, payloadHash)
	}

	result := base
	var mapping *EntityMapping

	switch operation.Type {
	case OperationTypeCreateExpense:
		if operation.CreateExpense == nil {
			result = failResult(result, ErrorCodeInvalidRequest, "payload is required", false)
			break
		}

		createdExpense, err := s.expenses.CreateExpense(ctx, expensesdomain.CreateExpenseInput{
			FamilyID:    input.FamilyID,
			UserID:      input.User.ID,
			Date:        operation.CreateExpense.Date,
			Amount:      operation.CreateExpense.Amount,
			Currency:    operation.CreateExpense.Currency,
			Title:       operation.CreateExpense.Title,
			CategoryIDs: operation.CreateExpense.CategoryIDs,
		})
		if err != nil {
			if errors.Is(err, expensesdomain.ErrCategoryNotFound) {
				result = failResult(result, ErrorCodeCategoryNotFound, "category not found", false)
				break
			}
			result = failResult(result, ErrorCodeInternalError, "internal error", true)
			break
		}

		result.Status = ResultStatusApplied
		result.LocalID = nonEmptyStringPtr(operation.LocalID)
		entity := EntityExpense
		result.Entity = &entity
		result.ServerID = nonEmptyStringPtr(createdExpense.ID)

		if result.LocalID != nil && result.ServerID != nil {
			mapping = &EntityMapping{
				Entity:   entity,
				LocalID:  *result.LocalID,
				ServerID: *result.ServerID,
			}
		}

	case OperationTypeCreateTodo:
		if operation.CreateTodo == nil {
			result = failResult(result, ErrorCodeInvalidRequest, "payload is required", false)
			break
		}

		createdTodo, err := s.todos.CreateTodoItem(ctx, input.FamilyID, todosdomain.CreateTodoItemInput{
			ListID: operation.CreateTodo.ListID,
			Title:  operation.CreateTodo.Title,
		})
		if err != nil {
			if errors.Is(err, todosdomain.ErrTodoListNotFound) {
				result = failResult(result, ErrorCodeTodoListNotFound, "todo list not found", false)
				break
			}
			result = failResult(result, ErrorCodeInternalError, "internal error", true)
			break
		}

		result.Status = ResultStatusApplied
		result.LocalID = nonEmptyStringPtr(operation.LocalID)
		entity := EntityTodoItem
		result.Entity = &entity
		result.ServerID = nonEmptyStringPtr(createdTodo.ID)

		if result.LocalID != nil && result.ServerID != nil {
			mapping = &EntityMapping{
				Entity:   entity,
				LocalID:  *result.LocalID,
				ServerID: *result.ServerID,
			}
		}

	case OperationTypeSetTodoCompleted:
		if operation.SetTodoCompleted == nil {
			result = failResult(result, ErrorCodeInvalidRequest, "payload is required", false)
			break
		}

		targetTodoID, resolveErr := s.resolveTodoID(ctx, input.FamilyID, input.User.ID, operation, localTodoIDs)
		if resolveErr != nil {
			result = failResult(result, ErrorCodeDependencyNotResolved, "todo id dependency is not resolved", false)
			break
		}

		var completedBy *todosdomain.UserSnapshot
		if operation.SetTodoCompleted.IsCompleted {
			completedBy = &todosdomain.UserSnapshot{
				ID:        input.User.ID,
				Name:      input.User.Name,
				Email:     input.User.Email,
				AvatarURL: input.User.AvatarURL,
			}
		}

		isCompleted := operation.SetTodoCompleted.IsCompleted
		_, err := s.todos.UpdateTodoItem(ctx, todosdomain.UpdateTodoItemInput{
			ID:          targetTodoID,
			FamilyID:    input.FamilyID,
			IsCompleted: &isCompleted,
			CompletedBy: completedBy,
		})
		if err != nil {
			if errors.Is(err, todosdomain.ErrTodoItemNotFound) {
				result = failResult(result, ErrorCodeTodoItemNotFound, "todo item not found", false)
				break
			}
			result = failResult(result, ErrorCodeInternalError, "internal error", true)
			break
		}

		result.Status = ResultStatusApplied

	default:
		result = failResult(result, ErrorCodeUnsupportedOperationType, "unsupported operation type", false)
	}

	updateRecord := *reserved
	if result.Status == ResultStatusApplied {
		updateRecord.Status = OperationStateApplied
		updateRecord.Entity = result.Entity
		updateRecord.ServerID = result.ServerID
		updateRecord.ErrorCode = nil
		updateRecord.ErrorMessage = nil
		updateRecord.Retryable = nil
	} else {
		updateRecord.Status = OperationStateFailed
		if result.Error != nil {
			code := result.Error.Code
			message := result.Error.Message
			retryable := result.Error.Retryable
			updateRecord.ErrorCode = &code
			updateRecord.ErrorMessage = &message
			updateRecord.Retryable = &retryable
		}
	}

	if result.LocalID != nil {
		updateRecord.LocalID = result.LocalID
	}

	if err := s.repo.UpdateOperation(ctx, &updateRecord); err != nil {
		return failResult(base, ErrorCodeInternalError, "internal error", true), nil
	}

	return result, mapping
}

func (s *Service) resolveTodoID(ctx context.Context, familyID, userID string, operation OperationInput, localTodoIDs map[string]string) (string, error) {
	if operation.SetTodoCompleted == nil {
		return "", fmt.Errorf("set_todo_completed payload is required")
	}

	if operation.SetTodoCompleted.TodoID != "" {
		return operation.SetTodoCompleted.TodoID, nil
	}

	localID := strings.TrimSpace(operation.SetTodoCompleted.TodoLocalID)
	if localID == "" {
		return "", fmt.Errorf("todo id is required")
	}

	if todoID := strings.TrimSpace(localTodoIDs[localID]); todoID != "" {
		return todoID, nil
	}

	todoID, found, err := s.repo.FindServerIDByLocalID(ctx, familyID, userID, EntityTodoItem, localID)
	if err != nil {
		return "", err
	}
	if !found || strings.TrimSpace(todoID) == "" {
		return "", fmt.Errorf("todo id dependency is not resolved")
	}

	return todoID, nil
}

func resultFromExisting(base OperationResult, operation OperationInput, existing *OperationRecord, payloadHash string) (OperationResult, *EntityMapping) {
	if existing == nil {
		return failResult(base, ErrorCodeBatchInProgress, "operation is being processed", true), nil
	}
	if existing.PayloadHash != payloadHash {
		return failResult(base, ErrorCodeOperationPayloadMismatch, "operation_id already used with different payload", false), nil
	}
	if existing.Status == OperationStatePending {
		return failResult(base, ErrorCodeBatchInProgress, "operation is being processed", true), nil
	}

	result := base
	if existing.Status == OperationStateFailed {
		result.Status = ResultStatusFailed
		if existing.ErrorCode != nil {
			result.Error = &OperationError{
				Code:      *existing.ErrorCode,
				Message:   valueOr(existing.ErrorMessage, "operation failed"),
				Retryable: valueOr(existing.Retryable, false),
			}
		} else {
			result.Error = &OperationError{
				Code:      ErrorCodeInternalError,
				Message:   "internal error",
				Retryable: true,
			}
		}
		result.LocalID = firstNonNil(existing.LocalID, nonEmptyStringPtr(operation.LocalID))
		return result, nil
	}

	result.Status = ResultStatusDuplicate
	result.LocalID = firstNonNil(existing.LocalID, nonEmptyStringPtr(operation.LocalID))
	result.Entity = cloneEntity(existing.Entity)
	result.ServerID = cloneString(existing.ServerID)

	if existing.ErrorCode != nil {
		result.Error = &OperationError{
			Code:      *existing.ErrorCode,
			Message:   valueOr(existing.ErrorMessage, "operation failed"),
			Retryable: valueOr(existing.Retryable, false),
		}
	}

	if result.LocalID != nil && result.ServerID != nil && result.Entity != nil {
		return result, &EntityMapping{
			Entity:   *result.Entity,
			LocalID:  *result.LocalID,
			ServerID: *result.ServerID,
		}
	}

	return result, nil
}

func failResult(base OperationResult, code ErrorCode, message string, retryable bool) OperationResult {
	base.Status = ResultStatusFailed
	base.Error = &OperationError{
		Code:      code,
		Message:   message,
		Retryable: retryable,
	}
	return base
}

func deriveBatchStatus(summary BatchSummary) BatchStatus {
	if summary.Failed == 0 {
		return BatchStatusSuccess
	}
	if summary.Applied > 0 || summary.Duplicate > 0 {
		return BatchStatusPartialSuccess
	}
	return BatchStatusFailed
}

func hashRequest(operations []OperationInput) (string, error) {
	hashes := make([]string, 0, len(operations))
	for _, operation := range operations {
		hash, err := hashOperation(operation)
		if err != nil {
			return "", err
		}
		hashes = append(hashes, hash)
	}
	return hashValue(hashes)
}

func hashOperation(operation OperationInput) (string, error) {
	var payload interface{}
	switch operation.Type {
	case OperationTypeCreateExpense:
		payload = operation.CreateExpense
	case OperationTypeCreateTodo:
		payload = operation.CreateTodo
	case OperationTypeSetTodoCompleted:
		payload = operation.SetTodoCompleted
	default:
		payload = map[string]string{"type": string(operation.Type)}
	}

	value := struct {
		Type    OperationType `json:"type"`
		LocalID string        `json:"local_id,omitempty"`
		Payload interface{}   `json:"payload"`
	}{
		Type:    operation.Type,
		LocalID: operation.LocalID,
		Payload: payload,
	}

	return hashValue(value)
}

func hashValue(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneEntity(value *Entity) *Entity {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func nonEmptyStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func firstNonNil[T any](values ...*T) *T {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func valueOr[T any](value *T, fallback T) T {
	if value == nil {
		return fallback
	}
	return *value
}
