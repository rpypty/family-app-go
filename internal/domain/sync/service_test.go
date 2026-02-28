package sync

import (
	"context"
	"fmt"
	stdsync "sync"
	"testing"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	todosdomain "family-app-go/internal/domain/todos"
)

func TestProcessBatchDuplicateOperationID(t *testing.T) {
	repo := newFakeSyncRepo()
	expensesSvc := newFakeExpensesService()
	todosSvc := newFakeTodosService()
	svc := NewService(repo, expensesSvc, todosSvc)

	input := BatchInput{
		FamilyID: "fam-1",
		User:     UserSnapshot{ID: "user-1", Name: "Test", Email: "test@example.com"},
		Operations: []OperationInput{
			{
				OperationID: "11111111-1111-4111-8111-111111111111",
				Type:        OperationTypeCreateTodo,
				LocalID:     "todo-local-1",
				CreateTodo: &CreateTodoPayload{
					ListID: "list-1",
					Title:  "Buy milk",
				},
			},
		},
	}

	first, err := svc.ProcessBatch(context.Background(), input)
	if err != nil {
		t.Fatalf("first process failed: %v", err)
	}
	if first.Results[0].Status != ResultStatusApplied {
		t.Fatalf("expected first status applied, got %s", first.Results[0].Status)
	}
	if todosSvc.createCalls != 1 {
		t.Fatalf("expected 1 todo create call, got %d", todosSvc.createCalls)
	}

	second, err := svc.ProcessBatch(context.Background(), input)
	if err != nil {
		t.Fatalf("second process failed: %v", err)
	}
	if second.Results[0].Status != ResultStatusDuplicate {
		t.Fatalf("expected second status duplicate, got %s", second.Results[0].Status)
	}
	if todosSvc.createCalls != 1 {
		t.Fatalf("expected no extra todo create call, got %d", todosSvc.createCalls)
	}
}

func TestProcessBatchRepeatWithIdempotencyKeyReturnsCachedResponse(t *testing.T) {
	repo := newFakeSyncRepo()
	expensesSvc := newFakeExpensesService()
	todosSvc := newFakeTodosService()
	svc := NewService(repo, expensesSvc, todosSvc)

	input := BatchInput{
		FamilyID:       "fam-1",
		User:           UserSnapshot{ID: "user-1", Name: "Test", Email: "test@example.com"},
		IdempotencyKey: "batch-key-123456",
		Operations: []OperationInput{
			{
				OperationID: "22222222-2222-4222-8222-222222222222",
				Type:        OperationTypeCreateTodo,
				LocalID:     "todo-local-2",
				CreateTodo: &CreateTodoPayload{
					ListID: "list-1",
					Title:  "Buy bread",
				},
			},
		},
	}

	first, err := svc.ProcessBatch(context.Background(), input)
	if err != nil {
		t.Fatalf("first process failed: %v", err)
	}
	second, err := svc.ProcessBatch(context.Background(), input)
	if err != nil {
		t.Fatalf("second process failed: %v", err)
	}

	if first.SyncID != second.SyncID {
		t.Fatalf("expected same sync_id for replay, got %s and %s", first.SyncID, second.SyncID)
	}
	if second.Results[0].Status != ResultStatusApplied {
		t.Fatalf("expected cached applied result, got %s", second.Results[0].Status)
	}
	if todosSvc.createCalls != 1 {
		t.Fatalf("expected single create call, got %d", todosSvc.createCalls)
	}
}

func TestProcessBatchPartialFail(t *testing.T) {
	repo := newFakeSyncRepo()
	expensesSvc := newFakeExpensesService()
	todosSvc := newFakeTodosService()
	svc := NewService(repo, expensesSvc, todosSvc)

	input := BatchInput{
		FamilyID: "fam-1",
		User:     UserSnapshot{ID: "user-1", Name: "Test", Email: "test@example.com"},
		Operations: []OperationInput{
			{
				OperationID: "33333333-3333-4333-8333-333333333333",
				Type:        OperationTypeCreateTodo,
				LocalID:     "todo-local-3",
				CreateTodo: &CreateTodoPayload{
					ListID: "list-1",
					Title:  "Buy apples",
				},
			},
			{
				OperationID: "44444444-4444-4444-8444-444444444444",
				Type:        OperationTypeSetTodoCompleted,
				SetTodoCompleted: &SetTodoCompletedPayload{
					TodoID:      "missing-todo-id",
					IsCompleted: true,
				},
			},
		},
	}

	response, err := svc.ProcessBatch(context.Background(), input)
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	if response.Status != BatchStatusPartialSuccess {
		t.Fatalf("expected partial_success, got %s", response.Status)
	}
	if response.Summary.Applied != 1 || response.Summary.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", response.Summary)
	}

	second := response.Results[1]
	if second.Status != ResultStatusFailed {
		t.Fatalf("expected failed status for second operation, got %s", second.Status)
	}
	if second.Error == nil || second.Error.Code != ErrorCodeTodoItemNotFound {
		t.Fatalf("expected todo_item_not_found error, got %+v", second.Error)
	}
}

func TestProcessBatchParallelSameOperationID(t *testing.T) {
	repo := newFakeSyncRepo()
	expensesSvc := newFakeExpensesService()
	todosSvc := newFakeTodosService()
	todosSvc.createDelay = 40 * time.Millisecond
	svc := NewService(repo, expensesSvc, todosSvc)

	input := BatchInput{
		FamilyID: "fam-1",
		User:     UserSnapshot{ID: "user-1", Name: "Test", Email: "test@example.com"},
		Operations: []OperationInput{
			{
				OperationID: "55555555-5555-4555-8555-555555555555",
				Type:        OperationTypeCreateTodo,
				LocalID:     "todo-local-race",
				CreateTodo: &CreateTodoPayload{
					ListID: "list-1",
					Title:  "Race",
				},
			},
		},
	}

	var wg stdsync.WaitGroup
	wg.Add(2)

	responses := make([]*BatchResponse, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		idx := i
		go func() {
			defer wg.Done()
			responses[idx], errs[idx] = svc.ProcessBatch(context.Background(), input)
		}()
	}
	wg.Wait()

	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("request %d failed: %v", i, errs[i])
		}
	}

	if todosSvc.createCalls != 1 {
		t.Fatalf("expected exactly one create call, got %d", todosSvc.createCalls)
	}

	applied := 0
	other := 0
	for _, response := range responses {
		status := response.Results[0].Status
		if status == ResultStatusApplied {
			applied++
		} else {
			other++
		}
	}

	if applied != 1 {
		t.Fatalf("expected one applied result, got %d", applied)
	}
	if other != 1 {
		t.Fatalf("expected one non-applied result, got %d", other)
	}
}

type fakeSyncRepo struct {
	mu stdsync.Mutex

	batchesByID  map[string]BatchRecord
	batchesByKey map[string]string

	operationsByID  map[string]OperationRecord
	operationsByKey map[string]string
}

func newFakeSyncRepo() *fakeSyncRepo {
	return &fakeSyncRepo{
		batchesByID:     make(map[string]BatchRecord),
		batchesByKey:    make(map[string]string),
		operationsByID:  make(map[string]OperationRecord),
		operationsByKey: make(map[string]string),
	}
}

func (r *fakeSyncRepo) BeginBatch(_ context.Context, batch *BatchRecord) (bool, *BatchRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if batch.IdempotencyKey == nil {
		copied := *batch
		r.batchesByID[copied.ID] = copied
		return true, nil, nil
	}

	key := batchKey(batch.FamilyID, batch.UserID, *batch.IdempotencyKey)
	if id, ok := r.batchesByKey[key]; ok {
		existing := r.batchesByID[id]
		copied := existing
		return false, &copied, nil
	}

	copied := *batch
	r.batchesByID[copied.ID] = copied
	r.batchesByKey[key] = copied.ID
	return true, nil, nil
}

func (r *fakeSyncRepo) CompleteBatch(_ context.Context, batchID string, status BatchState, responseJSON []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.batchesByID[batchID]
	if !ok {
		return nil
	}
	record.Status = status
	record.ResponseJSON = append([]byte{}, responseJSON...)
	r.batchesByID[batchID] = record
	return nil
}

func (r *fakeSyncRepo) ReserveOperation(_ context.Context, operation *OperationRecord) (bool, *OperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := operationKey(operation.FamilyID, operation.UserID, operation.OperationID)
	if id, ok := r.operationsByKey[key]; ok {
		existing := r.operationsByID[id]
		copied := existing
		return false, &copied, nil
	}

	copied := *operation
	r.operationsByID[copied.ID] = copied
	r.operationsByKey[key] = copied.ID
	return true, nil, nil
}

func (r *fakeSyncRepo) UpdateOperation(_ context.Context, operation *OperationRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, ok := r.operationsByID[operation.ID]
	if !ok {
		return nil
	}
	copied := *operation
	r.operationsByID[copied.ID] = copied
	return nil
}

func (r *fakeSyncRepo) FindServerIDByLocalID(_ context.Context, familyID, userID string, entity Entity, localID string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, operation := range r.operationsByID {
		if operation.FamilyID != familyID || operation.UserID != userID {
			continue
		}
		if operation.Entity == nil || *operation.Entity != entity {
			continue
		}
		if operation.LocalID == nil || *operation.LocalID != localID {
			continue
		}
		if operation.Status != OperationStateApplied || operation.ServerID == nil {
			continue
		}
		return *operation.ServerID, true, nil
	}

	return "", false, nil
}

func batchKey(familyID, userID, idempotencyKey string) string {
	return fmt.Sprintf("%s|%s|%s", familyID, userID, idempotencyKey)
}

func operationKey(familyID, userID, operationID string) string {
	return fmt.Sprintf("%s|%s|%s", familyID, userID, operationID)
}

type fakeExpensesService struct {
	mu          stdsync.Mutex
	createCalls int
	seq         int
}

func newFakeExpensesService() *fakeExpensesService {
	return &fakeExpensesService{}
}

func (f *fakeExpensesService) CreateExpense(_ context.Context, _ expensesdomain.CreateExpenseInput) (*expensesdomain.ExpenseWithCategories, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.createCalls++
	f.seq++
	id := fmt.Sprintf("expense-%d", f.seq)
	return &expensesdomain.ExpenseWithCategories{
		Expense: expensesdomain.Expense{ID: id},
	}, nil
}

type fakeTodosService struct {
	mu stdsync.Mutex

	createCalls int
	updateCalls int
	seq         int
	createDelay time.Duration

	lists map[string]struct{}
	items map[string]todosdomain.TodoItem
}

func newFakeTodosService() *fakeTodosService {
	return &fakeTodosService{
		lists: map[string]struct{}{
			"list-1": {},
		},
		items: make(map[string]todosdomain.TodoItem),
	}
}

func (f *fakeTodosService) CreateTodoItem(_ context.Context, _ string, input todosdomain.CreateTodoItemInput) (*todosdomain.TodoItem, error) {
	if f.createDelay > 0 {
		time.Sleep(f.createDelay)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.lists[input.ListID]; !ok {
		return nil, todosdomain.ErrTodoListNotFound
	}

	f.createCalls++
	f.seq++
	id := fmt.Sprintf("todo-%d", f.seq)
	item := todosdomain.TodoItem{
		ID:     id,
		ListID: input.ListID,
		Title:  input.Title,
	}
	f.items[id] = item
	copied := item
	return &copied, nil
}

func (f *fakeTodosService) UpdateTodoItem(_ context.Context, input todosdomain.UpdateTodoItemInput) (*todosdomain.TodoItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	item, ok := f.items[input.ID]
	if !ok {
		return nil, todosdomain.ErrTodoItemNotFound
	}

	f.updateCalls++
	if input.IsCompleted != nil {
		item.IsCompleted = *input.IsCompleted
	}
	f.items[input.ID] = item
	copied := item
	return &copied, nil
}
