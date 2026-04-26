package receipts

import (
	"context"
	"errors"
	"testing"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
)

const (
	testFamilyID   = "11111111-1111-1111-1111-111111111111"
	testUserID     = "22222222-2222-2222-2222-222222222222"
	testJobID      = "33333333-3333-3333-3333-333333333333"
	testDraftID    = "44444444-4444-4444-4444-444444444444"
	testCategoryID = "55555555-5555-5555-5555-555555555555"
)

var errUpdateJobFailed = errors.New("update job failed")

var validPNGBytes = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d}

func TestProcessNextRecoversQueuedJobFromPersistedFile(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	expenseRepo := newFakeReceiptExpenseRepo()
	receiptRepo.expenseRepo = expenseRepo
	fileStore := newMemoryReceiptFileStore()
	categoryProvider := fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
		},
	}

	service := NewServiceWithOptions(receiptRepo, fakeParser{}, categoryProvider, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     fileStore,
		WorkerEnabled: false,
		WorkerID:      "test-worker",
	})

	job, err := service.CreateParse(ctx, CreateParseInput{
		FamilyID:            testFamilyID,
		UserID:              testUserID,
		CategoryMode:        CategoryModeSelected,
		SelectedCategoryIDs: []string{testCategoryID},
		RequestedCurrency:   "BYN",
		File: UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(validPNGBytes)),
			SHA256:      "sha",
			Data:        validPNGBytes,
		},
	})
	if err != nil {
		t.Fatalf("create parse: %v", err)
	}

	files, err := receiptRepo.ListFilesByJobID(ctx, job.ID)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) != 1 || files[0].StorageKey == nil || *files[0].StorageKey == "" {
		t.Fatalf("expected persisted storage key, got %#v", files)
	}

	processed, err := service.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process next: %v", err)
	}
	if !processed {
		t.Fatal("expected queued job to be processed")
	}

	ready, err := service.GetParse(ctx, testFamilyID, job.ID)
	if err != nil {
		t.Fatalf("get parse: %v", err)
	}
	if ready.Status != StatusReady {
		t.Fatalf("expected ready job, got %s", ready.Status)
	}
	if len(ready.DraftExpenses) != 1 {
		t.Fatalf("expected one draft expense, got %d", len(ready.DraftExpenses))
	}
}

func TestRecoverStaleProcessingJobsRequeuesOldLocks(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	lockedAt := time.Now().UTC().Add(-2 * time.Hour)
	receiptRepo.jobs[testJobID] = &Job{
		ID:        testJobID,
		FamilyID:  testFamilyID,
		UserID:    testUserID,
		Status:    StatusProcessing,
		LockedAt:  &lockedAt,
		LockedBy:  stringPtr("old-worker"),
		UpdatedAt: lockedAt,
	}

	service := NewServiceWithOptions(receiptRepo, fakeParser{}, fakeCategoryProvider{}, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     newMemoryReceiptFileStore(),
		WorkerEnabled: false,
		WorkerID:      "test-worker",
		StaleAfter:    time.Hour,
	})

	if err := service.RecoverStaleProcessing(ctx); err != nil {
		t.Fatalf("recover stale processing: %v", err)
	}
	job := receiptRepo.jobs[testJobID]
	if job.Status != StatusQueued {
		t.Fatalf("expected stale processing job to be queued, got %s", job.Status)
	}
	if job.LockedAt != nil || job.LockedBy != nil {
		t.Fatalf("expected lock metadata to be cleared, got locked_at=%v locked_by=%v", job.LockedAt, job.LockedBy)
	}
}

func TestApproveParseRollsBackExpensesWhenReceiptUpdateFails(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	expenseRepo := newFakeReceiptExpenseRepo()
	receiptRepo.expenseRepo = expenseRepo
	receiptRepo.failUpdateJob = true
	receiptRepo.jobs[testJobID] = &Job{
		ID:       testJobID,
		FamilyID: testFamilyID,
		UserID:   testUserID,
		Status:   StatusReady,
	}
	receiptRepo.drafts[testJobID] = []DraftExpense{
		{
			ID:         testDraftID,
			JobID:      testJobID,
			Title:      "Products",
			Amount:     10,
			Currency:   "BYN",
			CategoryID: testCategoryID,
			Warnings:   []byte("[]"),
		},
	}

	service := NewServiceWithOptions(receiptRepo, nil, nil, fakeExpenseBatchCreator{}, ServiceOptions{WorkerEnabled: false})
	date := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	_, err := service.ApproveParse(ctx, ApproveInput{
		FamilyID:     testFamilyID,
		UserID:       testUserID,
		BaseCurrency: "BYN",
		JobID:        testJobID,
		Expenses: []ApproveExpenseInput{
			{
				DraftID:     testDraftID,
				Date:        date,
				Title:       "Products",
				Amount:      10,
				Currency:    "BYN",
				CategoryIDs: []string{testCategoryID},
			},
		},
	})

	if !errors.Is(err, errUpdateJobFailed) {
		t.Fatalf("expected update job error, got %v", err)
	}
	if len(expenseRepo.expenses) != 0 {
		t.Fatalf("expected expenses rollback, got %d expenses", len(expenseRepo.expenses))
	}
	if receiptRepo.jobs[testJobID].Status != StatusReady {
		t.Fatalf("expected receipt job status rollback to ready, got %s", receiptRepo.jobs[testJobID].Status)
	}
}

func TestApproveParseDeletesStoredFilesAfterSuccess(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	expenseRepo := newFakeReceiptExpenseRepo()
	receiptRepo.expenseRepo = expenseRepo
	fileStore := newMemoryReceiptFileStore()
	storageKey := testJobID + "/file-1"
	fileStore.files[storageKey] = []byte("receipt-bytes")
	receiptRepo.jobs[testJobID] = &Job{
		ID:       testJobID,
		FamilyID: testFamilyID,
		UserID:   testUserID,
		Status:   StatusReady,
	}
	receiptRepo.files[testJobID] = []File{{
		ID:         "file-1",
		JobID:      testJobID,
		StorageKey: &storageKey,
	}}
	receiptRepo.items[testJobID] = []Item{{
		ID:              "item-1",
		JobID:           testJobID,
		RawName:         "Milk",
		LineTotal:       10,
		FinalLineTotal:  floatPtr(10),
		FinalCategoryID: stringPtr(testCategoryID),
	}}
	receiptRepo.drafts[testJobID] = []DraftExpense{
		{
			ID:         testDraftID,
			JobID:      testJobID,
			Title:      "Products",
			Amount:     10,
			Currency:   "BYN",
			CategoryID: testCategoryID,
			Warnings:   []byte("[]"),
		},
	}

	service := NewServiceWithOptions(receiptRepo, nil, nil, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     fileStore,
		WorkerEnabled: false,
	})
	date := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	created, err := service.ApproveParse(ctx, ApproveInput{
		FamilyID:     testFamilyID,
		UserID:       testUserID,
		BaseCurrency: "BYN",
		JobID:        testJobID,
		Expenses: []ApproveExpenseInput{{
			DraftID:     testDraftID,
			Date:        date,
			Title:       "Products",
			Amount:      10,
			Currency:    "BYN",
			CategoryIDs: []string{testCategoryID},
		}},
	})
	if err != nil {
		t.Fatalf("approve parse: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created expense, got %d", len(created))
	}
	if _, ok := fileStore.files[storageKey]; ok {
		t.Fatal("expected stored receipt file to be deleted after approve")
	}
}

func TestCancelParseDeletesStoredFilesAfterSuccess(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	fileStore := newMemoryReceiptFileStore()
	storageKey := testJobID + "/file-1"
	fileStore.files[storageKey] = []byte("receipt-bytes")
	receiptRepo.jobs[testJobID] = &Job{
		ID:       testJobID,
		FamilyID: testFamilyID,
		UserID:   testUserID,
		Status:   StatusQueued,
	}
	receiptRepo.files[testJobID] = []File{{
		ID:         "file-1",
		JobID:      testJobID,
		StorageKey: &storageKey,
	}}

	service := NewServiceWithOptions(receiptRepo, nil, nil, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     fileStore,
		WorkerEnabled: false,
	})

	job, err := service.CancelParse(ctx, testFamilyID, testJobID)
	if err != nil {
		t.Fatalf("cancel parse: %v", err)
	}
	if job.Status != StatusCancelled {
		t.Fatalf("expected cancelled status, got %s", job.Status)
	}
	if _, ok := fileStore.files[storageKey]; ok {
		t.Fatal("expected stored receipt file to be deleted after cancel")
	}
}

func TestProcessNextMarksInvalidParserOutputAsInvalidResponse(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	fileStore := newMemoryReceiptFileStore()
	categoryProvider := fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
		},
	}

	service := NewServiceWithOptions(receiptRepo, invalidResponseParser{}, categoryProvider, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     fileStore,
		WorkerEnabled: false,
		WorkerID:      "test-worker",
	})

	job, err := service.CreateParse(ctx, CreateParseInput{
		FamilyID:            testFamilyID,
		UserID:              testUserID,
		CategoryMode:        CategoryModeSelected,
		SelectedCategoryIDs: []string{testCategoryID},
		RequestedCurrency:   "BYN",
		File: UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(validPNGBytes)),
			SHA256:      "sha",
			Data:        validPNGBytes,
		},
	})
	if err != nil {
		t.Fatalf("create parse: %v", err)
	}

	processed, err := service.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process next: %v", err)
	}
	if !processed {
		t.Fatal("expected queued job to be processed")
	}

	updatedJob, err := service.GetParse(ctx, testFamilyID, job.ID)
	if err != nil {
		t.Fatalf("get parse: %v", err)
	}
	if updatedJob.Status != StatusFailed {
		t.Fatalf("expected failed job, got %s", updatedJob.Status)
	}
	if updatedJob.ErrorCode == nil || *updatedJob.ErrorCode != "llm_invalid_response" {
		t.Fatalf("expected llm_invalid_response, got %#v", updatedJob.ErrorCode)
	}
}

func TestProcessNextKeepsUnresolvedItemsAndReadyStatus(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	fileStore := newMemoryReceiptFileStore()
	categoryProvider := fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
		},
	}

	service := NewServiceWithOptions(receiptRepo, mixedCategoryParser{}, categoryProvider, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     fileStore,
		WorkerEnabled: false,
		WorkerID:      "test-worker",
	})

	job, err := service.CreateParse(ctx, CreateParseInput{
		FamilyID:            testFamilyID,
		UserID:              testUserID,
		CategoryMode:        CategoryModeSelected,
		SelectedCategoryIDs: []string{testCategoryID},
		RequestedCurrency:   "BYN",
		File: UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(validPNGBytes)),
			SHA256:      "sha",
			Data:        validPNGBytes,
		},
	})
	if err != nil {
		t.Fatalf("create parse: %v", err)
	}

	processed, err := service.ProcessNext(ctx)
	if err != nil {
		t.Fatalf("process next: %v", err)
	}
	if !processed {
		t.Fatal("expected queued job to be processed")
	}

	parse, err := service.GetParse(ctx, testFamilyID, job.ID)
	if err != nil {
		t.Fatalf("get parse: %v", err)
	}
	if parse.Status != StatusReady {
		t.Fatalf("expected ready job, got %s", parse.Status)
	}
	if len(parse.DraftExpenses) != 1 {
		t.Fatalf("expected one draft expense, got %d", len(parse.DraftExpenses))
	}
	if len(parse.Items) != 2 {
		t.Fatalf("expected two items, got %d", len(parse.Items))
	}
	if !hasUnresolvedItems(parse.Items) {
		t.Fatal("expected unresolved item to remain for manual review")
	}
}

func TestUpdateItemsRebuildsDraftsAndResolvesUncategorizedItem(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	receiptRepo.jobs[testJobID] = &Job{
		ID:                  testJobID,
		FamilyID:            testFamilyID,
		UserID:              testUserID,
		Status:              StatusReady,
		CategoryMode:        CategoryModeSelected,
		SelectedCategoryIDs: []byte(`["` + testCategoryID + `"]`),
		Currency:            stringPtr("BYN"),
	}
	amountA := 10.0
	amountB := 3.0
	receiptRepo.items[testJobID] = []Item{
		{
			ID:              "item-resolved",
			JobID:           testJobID,
			RawName:         "Milk",
			LineTotal:       amountA,
			FinalLineTotal:  &amountA,
			LLMCategoryID:   stringPtr(testCategoryID),
			FinalCategoryID: stringPtr(testCategoryID),
		},
		{
			ID:             "item-unresolved",
			JobID:          testJobID,
			RawName:        "Bread",
			LineTotal:      amountB,
			FinalLineTotal: &amountB,
		},
	}
	receiptRepo.drafts[testJobID] = []DraftExpense{
		{
			ID:         testDraftID,
			JobID:      testJobID,
			Title:      "Products",
			Amount:     amountA,
			Currency:   "BYN",
			CategoryID: testCategoryID,
			Warnings:   []byte("[]"),
		},
	}

	service := NewServiceWithOptions(receiptRepo, nil, fakeCategoryProvider{
		categories: []expensesdomain.Category{{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"}},
	}, fakeExpenseBatchCreator{}, ServiceOptions{WorkerEnabled: false})

	newAmount := 5.5
	updated, err := service.UpdateItems(ctx, UpdateItemsInput{
		FamilyID: testFamilyID,
		JobID:    testJobID,
		Items: []ReviewItemInput{
			{ItemID: "item-unresolved", Amount: &newAmount, CategoryID: stringPtr(testCategoryID)},
		},
	})
	if err != nil {
		t.Fatalf("update items: %v", err)
	}
	if hasUnresolvedItems(updated.Items) {
		t.Fatal("expected unresolved items to be resolved after update")
	}
	if len(updated.DraftExpenses) != 1 {
		t.Fatalf("expected one rebuilt draft expense, got %d", len(updated.DraftExpenses))
	}
	if updated.DraftExpenses[0].Amount != 15.5 {
		t.Fatalf("expected rebuilt draft amount 15.5, got %v", updated.DraftExpenses[0].Amount)
	}
}

type fakeExpenseBatchCreator struct{}

func (fakeExpenseBatchCreator) CreateExpensesBatch(context.Context, []expensesdomain.CreateExpenseInput) ([]expensesdomain.ExpenseWithCategories, error) {
	return nil, errors.New("unexpected CreateExpensesBatch call")
}

func (fakeExpenseBatchCreator) CreateExpensesBatchWithRepository(ctx context.Context, repo expensesdomain.Repository, inputs []expensesdomain.CreateExpenseInput) ([]expensesdomain.ExpenseWithCategories, error) {
	result := make([]expensesdomain.ExpenseWithCategories, 0, len(inputs))
	for index, input := range inputs {
		expense := expensesdomain.Expense{
			ID:       "expense-" + string(rune('1'+index)),
			FamilyID: input.FamilyID,
			UserID:   input.UserID,
			Date:     input.Date,
			Amount:   input.Amount,
			Currency: input.Currency,
			Title:    input.Title,
		}
		if err := repo.CreateExpense(ctx, &expense); err != nil {
			return nil, err
		}
		if err := repo.ReplaceExpenseCategories(ctx, expense.ID, input.CategoryIDs); err != nil {
			return nil, err
		}
		result = append(result, expensesdomain.ExpenseWithCategories{
			Expense:     expense,
			CategoryIDs: append([]string{}, input.CategoryIDs...),
		})
	}
	return result, nil
}

type fakeParser struct{}

func (fakeParser) ParseReceipt(_ context.Context, input ParseReceiptInput) (*ParsedReceipt, error) {
	if len(input.Categories) == 0 {
		return nil, ErrCategorySelectionRequired
	}
	categoryID := input.Categories[0].ID
	confidence := 0.9
	total := 10.0
	return &ParsedReceipt{
		Currency:      "BYN",
		DetectedTotal: &total,
		Provider:      "fake",
		Model:         "fake",
		RawResponse:   []byte(`{"fake":true}`),
		Items: []ParsedItem{
			{
				RawName:            "Receipt item",
				LineTotal:          total,
				CategoryID:         &categoryID,
				CategoryConfidence: &confidence,
			},
		},
	}, nil
}

type invalidResponseParser struct{}

func (invalidResponseParser) ParseReceipt(context.Context, ParseReceiptInput) (*ParsedReceipt, error) {
	return nil, ErrLLMInvalidResponse
}

type mixedCategoryParser struct{}

func (mixedCategoryParser) ParseReceipt(_ context.Context, input ParseReceiptInput) (*ParsedReceipt, error) {
	if len(input.Categories) == 0 {
		return nil, ErrCategorySelectionRequired
	}
	categoryID := input.Categories[0].ID
	confidence := 0.9
	amountA := 10.0
	amountB := 4.0
	return &ParsedReceipt{
		Currency:    "BYN",
		Provider:    "fake",
		Model:       "fake",
		RawResponse: []byte(`{"fake":true}`),
		Items: []ParsedItem{
			{RawName: "Milk", LineTotal: amountA, CategoryID: &categoryID, CategoryConfidence: &confidence},
			{RawName: "Bread", LineTotal: amountB},
		},
	}, nil
}

func floatPtr(value float64) *float64 {
	return &value
}

type fakeCategoryProvider struct {
	categories []expensesdomain.Category
}

func (p fakeCategoryProvider) ListCategories(context.Context, string) ([]expensesdomain.Category, error) {
	return append([]expensesdomain.Category{}, p.categories...), nil
}

type memoryReceiptFileStore struct {
	files map[string][]byte
}

func newMemoryReceiptFileStore() *memoryReceiptFileStore {
	return &memoryReceiptFileStore{files: make(map[string][]byte)}
}

func (s *memoryReceiptFileStore) Save(_ context.Context, jobID, fileID string, file UploadedFile) (string, error) {
	key := jobID + "/" + fileID
	s.files[key] = append([]byte{}, file.Data...)
	return key, nil
}

func (s *memoryReceiptFileStore) Load(_ context.Context, storageKey string) ([]byte, error) {
	data, ok := s.files[storageKey]
	if !ok {
		return nil, ErrInvalidReceiptFile
	}
	return append([]byte{}, data...), nil
}

func (s *memoryReceiptFileStore) Delete(_ context.Context, storageKey string) error {
	delete(s.files, storageKey)
	return nil
}

type fakeReceiptRepo struct {
	jobs          map[string]*Job
	files         map[string][]File
	items         map[string][]Item
	drafts        map[string][]DraftExpense
	expenseRepo   *fakeReceiptExpenseRepo
	failUpdateJob bool
}

func newFakeReceiptRepo() *fakeReceiptRepo {
	return &fakeReceiptRepo{
		jobs:   make(map[string]*Job),
		files:  make(map[string][]File),
		items:  make(map[string][]Item),
		drafts: make(map[string][]DraftExpense),
	}
}

func (r *fakeReceiptRepo) Transaction(ctx context.Context, fn func(Repository, expensesdomain.Repository) error) error {
	tx := r.clone()
	tx.expenseRepo = r.expenseRepo.clone()
	if err := fn(tx, tx.expenseRepo); err != nil {
		return err
	}
	r.jobs = tx.jobs
	r.files = tx.files
	r.items = tx.items
	r.drafts = tx.drafts
	r.expenseRepo.expenses = tx.expenseRepo.expenses
	r.expenseRepo.expenseCategories = tx.expenseRepo.expenseCategories
	return nil
}

func (r *fakeReceiptRepo) clone() *fakeReceiptRepo {
	clone := newFakeReceiptRepo()
	clone.failUpdateJob = r.failUpdateJob
	for id, job := range r.jobs {
		jobCopy := *job
		clone.jobs[id] = &jobCopy
	}
	for jobID, files := range r.files {
		clone.files[jobID] = append([]File{}, files...)
	}
	for jobID, items := range r.items {
		clone.items[jobID] = append([]Item{}, items...)
	}
	for jobID, drafts := range r.drafts {
		clone.drafts[jobID] = append([]DraftExpense{}, drafts...)
	}
	return clone
}

func (r *fakeReceiptRepo) CreateJob(_ context.Context, job *Job) error {
	jobCopy := *job
	r.jobs[job.ID] = &jobCopy
	return nil
}

func (r *fakeReceiptRepo) CreateFile(_ context.Context, file *File) error {
	r.files[file.JobID] = append(r.files[file.JobID], *file)
	return nil
}

func (r *fakeReceiptRepo) GetJobByID(_ context.Context, familyID, jobID string) (*Job, error) {
	job, ok := r.jobs[jobID]
	if !ok || job.FamilyID != familyID {
		return nil, ErrReceiptParseNotFound
	}
	jobCopy := *job
	return &jobCopy, nil
}

func (r *fakeReceiptRepo) GetActiveJob(context.Context, string) (*Job, error) {
	return nil, nil
}

func (r *fakeReceiptRepo) CountActiveJobs(_ context.Context, familyID string) (int64, error) {
	var count int64
	for _, job := range r.jobs {
		if job.FamilyID == familyID && isActiveStatus(job.Status) {
			count++
		}
	}
	return count, nil
}

func (r *fakeReceiptRepo) AcquireQueuedJob(_ context.Context, workerID string, now time.Time) (*Job, error) {
	for _, job := range r.jobs {
		if job.Status != StatusQueued {
			continue
		}
		if job.NextAttemptAt != nil && job.NextAttemptAt.After(now) {
			continue
		}
		job.Status = StatusProcessing
		job.AttemptCount++
		job.LastAttemptAt = &now
		job.LockedAt = &now
		job.LockedBy = &workerID
		job.UpdatedAt = now
		jobCopy := *job
		return &jobCopy, nil
	}
	return nil, nil
}

func (r *fakeReceiptRepo) RequeueStaleProcessing(_ context.Context, staleBefore time.Time) (int64, error) {
	var count int64
	for _, job := range r.jobs {
		if job.Status != StatusProcessing || job.LockedAt == nil || !job.LockedAt.Before(staleBefore) {
			continue
		}
		job.Status = StatusQueued
		job.LockedAt = nil
		job.LockedBy = nil
		job.UpdatedAt = time.Now().UTC()
		count++
	}
	return count, nil
}

func (r *fakeReceiptRepo) UpdateJob(_ context.Context, job *Job) error {
	if r.failUpdateJob && job.Status == StatusApproved {
		return errUpdateJobFailed
	}
	jobCopy := *job
	r.jobs[job.ID] = &jobCopy
	return nil
}

func (r *fakeReceiptRepo) ListFilesByJobID(_ context.Context, jobID string) ([]File, error) {
	return append([]File{}, r.files[jobID]...), nil
}

func (r *fakeReceiptRepo) ListItemsByJobID(_ context.Context, jobID string) ([]Item, error) {
	return append([]Item{}, r.items[jobID]...), nil
}

func (r *fakeReceiptRepo) ReplaceItems(_ context.Context, jobID string, items []Item) error {
	r.items[jobID] = append([]Item{}, items...)
	return nil
}

func (r *fakeReceiptRepo) ReplaceDraftExpenses(_ context.Context, jobID string, drafts []DraftExpense) error {
	r.drafts[jobID] = append([]DraftExpense{}, drafts...)
	return nil
}

func (r *fakeReceiptRepo) ListDraftExpenses(_ context.Context, jobID string) ([]DraftExpense, error) {
	return append([]DraftExpense{}, r.drafts[jobID]...), nil
}

func (r *fakeReceiptRepo) UpdateDraftExpense(_ context.Context, draft *DraftExpense) error {
	drafts := r.drafts[draft.JobID]
	for index := range drafts {
		if drafts[index].ID == draft.ID {
			drafts[index] = *draft
			r.drafts[draft.JobID] = drafts
			return nil
		}
	}
	return ErrReceiptParseInvalidStatus
}

func (r *fakeReceiptRepo) UpdateItem(_ context.Context, item *Item) error {
	items := r.items[item.JobID]
	for index := range items {
		if items[index].ID == item.ID {
			items[index] = *item
			r.items[item.JobID] = items
			return nil
		}
	}
	return ErrReceiptParseInvalidStatus
}

type fakeReceiptExpenseRepo struct {
	expenses          map[string]*expensesdomain.Expense
	expenseCategories map[string][]string
}

func newFakeReceiptExpenseRepo() *fakeReceiptExpenseRepo {
	return &fakeReceiptExpenseRepo{
		expenses:          make(map[string]*expensesdomain.Expense),
		expenseCategories: make(map[string][]string),
	}
}

func (r *fakeReceiptExpenseRepo) clone() *fakeReceiptExpenseRepo {
	clone := newFakeReceiptExpenseRepo()
	for id, expense := range r.expenses {
		expenseCopy := *expense
		clone.expenses[id] = &expenseCopy
	}
	for expenseID, categoryIDs := range r.expenseCategories {
		clone.expenseCategories[expenseID] = append([]string{}, categoryIDs...)
	}
	return clone
}

func (r *fakeReceiptExpenseRepo) Transaction(ctx context.Context, fn func(expensesdomain.Repository) error) error {
	return fn(r)
}

func (r *fakeReceiptExpenseRepo) ListExpenses(context.Context, string, expensesdomain.ListFilter) ([]expensesdomain.Expense, int64, error) {
	return nil, 0, nil
}

func (r *fakeReceiptExpenseRepo) GetExpenseByID(context.Context, string, string) (*expensesdomain.Expense, error) {
	return nil, expensesdomain.ErrExpenseNotFound
}

func (r *fakeReceiptExpenseRepo) CreateExpense(_ context.Context, expense *expensesdomain.Expense) error {
	expenseCopy := *expense
	r.expenses[expense.ID] = &expenseCopy
	return nil
}

func (r *fakeReceiptExpenseRepo) UpdateExpense(context.Context, *expensesdomain.Expense) error {
	return nil
}

func (r *fakeReceiptExpenseRepo) DeleteExpense(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *fakeReceiptExpenseRepo) ReplaceExpenseCategories(_ context.Context, expenseID string, categoryIDs []string) error {
	r.expenseCategories[expenseID] = append([]string{}, categoryIDs...)
	return nil
}

func (r *fakeReceiptExpenseRepo) GetCategoryIDsByExpenseIDs(context.Context, []string) (map[string][]string, error) {
	return nil, nil
}

func (r *fakeReceiptExpenseRepo) CountCategoriesByIDs(context.Context, string, []string) (int64, error) {
	return 0, nil
}

func (r *fakeReceiptExpenseRepo) ListCategories(context.Context, string) ([]expensesdomain.Category, error) {
	return nil, nil
}

func (r *fakeReceiptExpenseRepo) CreateCategory(context.Context, *expensesdomain.Category) error {
	return nil
}

func (r *fakeReceiptExpenseRepo) GetCategoryByID(context.Context, string, string) (*expensesdomain.Category, error) {
	return nil, expensesdomain.ErrCategoryNotFound
}

func (r *fakeReceiptExpenseRepo) UpdateCategory(context.Context, *expensesdomain.Category) error {
	return nil
}

func (r *fakeReceiptExpenseRepo) CountCategoriesByName(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (r *fakeReceiptExpenseRepo) DeleteCategory(context.Context, string, string) (bool, error) {
	return false, nil
}

func (r *fakeReceiptExpenseRepo) CountExpenseCategoriesByCategoryID(context.Context, string) (int64, error) {
	return 0, nil
}
