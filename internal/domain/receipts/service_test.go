package receipts

import (
	"context"
	"errors"
	"sort"
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
	testSportID    = "66666666-6666-6666-6666-666666666666"
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

func TestApproveParseCreatesCorrectionEventForChangedCategory(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newReadyApproveServiceWithItem(Item{
		ID:                    "item-1",
		JobID:                 testJobID,
		RawName:               "Exponenta cocktail",
		NormalizedName:        stringPtr("Exponenta cocktail"),
		LineTotal:             10,
		FinalLineTotal:        floatPtr(10),
		LLMCategoryID:         stringPtr(testCategoryID),
		FinalCategoryID:       stringPtr(testSportID),
		LLMCategoryConfidence: floatPtr(0.6),
	})
	date := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	_, err := service.ApproveParse(ctx, ApproveInput{
		FamilyID:     testFamilyID,
		UserID:       testUserID,
		BaseCurrency: "BYN",
		JobID:        testJobID,
		Expenses: []ApproveExpenseInput{{
			DraftID:     testDraftID,
			Date:        date,
			Title:       "Sport",
			Amount:      10,
			Currency:    "BYN",
			CategoryIDs: []string{testSportID},
		}},
	})
	if err != nil {
		t.Fatalf("approve parse: %v", err)
	}
	if len(receiptRepo.correctionEvents) != 1 {
		t.Fatalf("expected 1 correction event, got %d", len(receiptRepo.correctionEvents))
	}
	event := receiptRepo.correctionEvents[0]
	if event.SourceItemText != "Exponenta cocktail" || event.NormalizedItemText != "Exponenta cocktail" {
		t.Fatalf("unexpected event text %+v", event)
	}
	if event.LLMCategoryID == nil || *event.LLMCategoryID != testCategoryID || event.FinalCategoryID != testSportID {
		t.Fatalf("unexpected event categories %+v", event)
	}
	if len(receiptRepo.hints) != 0 {
		t.Fatalf("expected approve to defer hint materialization, got %+v", receiptRepo.hints)
	}
	if len(receiptRepo.hintExamples) != 0 {
		t.Fatalf("expected approve to defer hint examples, got %d", len(receiptRepo.hintExamples))
	}
}

func TestApproveParseCreatesCorrectionEventForManuallyCategorizedUnresolvedItem(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newReadyApproveServiceWithItem(Item{
		ID:              "item-1",
		JobID:           testJobID,
		RawName:         "Bombbar",
		LineTotal:       10,
		FinalLineTotal:  floatPtr(10),
		FinalCategoryID: stringPtr(testSportID),
	})
	date := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	_, err := service.ApproveParse(ctx, ApproveInput{
		FamilyID:     testFamilyID,
		UserID:       testUserID,
		BaseCurrency: "BYN",
		JobID:        testJobID,
		Expenses: []ApproveExpenseInput{{
			DraftID:     testDraftID,
			Date:        date,
			Title:       "Sport",
			Amount:      10,
			Currency:    "BYN",
			CategoryIDs: []string{testSportID},
		}},
	})
	if err != nil {
		t.Fatalf("approve parse: %v", err)
	}
	if len(receiptRepo.correctionEvents) != 1 {
		t.Fatalf("expected correction event, got %d", len(receiptRepo.correctionEvents))
	}
	if receiptRepo.correctionEvents[0].LLMCategoryID != nil {
		t.Fatalf("expected nil LLM category, got %+v", receiptRepo.correctionEvents[0].LLMCategoryID)
	}
	if receiptRepo.correctionEvents[0].NormalizedItemText != "Bombbar" {
		t.Fatalf("expected raw name fallback, got %+v", receiptRepo.correctionEvents[0])
	}
}

func TestApproveParseSkipsCorrectionEventWhenCategoryUnchanged(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newReadyApproveServiceWithItem(Item{
		ID:              "item-1",
		JobID:           testJobID,
		RawName:         "Milk",
		LineTotal:       10,
		FinalLineTotal:  floatPtr(10),
		LLMCategoryID:   stringPtr(testCategoryID),
		FinalCategoryID: stringPtr(testCategoryID),
	})
	date := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)

	_, err := service.ApproveParse(ctx, ApproveInput{
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
	if len(receiptRepo.correctionEvents) != 0 {
		t.Fatalf("expected no correction events, got %+v", receiptRepo.correctionEvents)
	}
}

func TestMaterializeNextCategoryCorrectionMatchesExistingHintAndIncrementsCount(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	receiptRepo.correctionEvents = []CategoryCorrectionEvent{{
		ID:                 "event-1",
		FamilyID:           testFamilyID,
		UserID:             testUserID,
		ReceiptParseJobID:  testJobID,
		ReceiptParseItemID: "item-1",
		SourceItemText:     "Exponenta strawberry",
		NormalizedItemText: "Exponenta strawberry",
		LLMCategoryID:      stringPtr(testCategoryID),
		FinalCategoryID:    testSportID,
		CreatedAt:          time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
	}}
	receiptRepo.hints = []FamilyHint{{
		ID:              "hint-1",
		FamilyID:        testFamilyID,
		CanonicalName:   "Exponenta cocktail",
		FinalCategoryID: testSportID,
		TimesConfirmed:  1,
		LastConfirmedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
	}}
	normalizer := &fakeHintNormalizer{
		result: &NormalizeCategoryCorrectionResult{
			Action:     NormalizeActionMatchExisting,
			HintID:     stringPtr("hint-1"),
			Confidence: 0.95,
		},
	}
	service := NewServiceWithOptions(receiptRepo, nil, fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
			{ID: testSportID, FamilyID: testFamilyID, Name: "Sport"},
		},
	}, fakeExpenseBatchCreator{}, ServiceOptions{
		HintNormalizer: normalizer,
		WorkerEnabled:  false,
	})

	processed, err := service.MaterializeNextCategoryCorrection(ctx)
	if err != nil {
		t.Fatalf("materialize correction: %v", err)
	}
	if !processed {
		t.Fatal("expected correction event to be processed")
	}
	if len(receiptRepo.hints) != 1 || receiptRepo.hints[0].TimesConfirmed != 2 {
		t.Fatalf("expected existing hint count to increment, got %+v", receiptRepo.hints)
	}
	if len(receiptRepo.hintExamples) != 1 || receiptRepo.hintExamples[0].HintID != "hint-1" {
		t.Fatalf("expected hint example for matched hint, got %+v", receiptRepo.hintExamples)
	}
	if receiptRepo.correctionEvents[0].ProcessedAt == nil {
		t.Fatalf("expected event to be marked processed, got %+v", receiptRepo.correctionEvents[0])
	}
}

func TestMaterializeNextCategoryCorrectionCreatesNewHintFromLLMCanonicalName(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newMaterializerServiceWithEvent(&fakeHintNormalizer{
		result: &NormalizeCategoryCorrectionResult{
			Action:        NormalizeActionCreateNew,
			CanonicalName: "Exponenta cocktail",
			Confidence:    0.92,
		},
	})

	processed, err := service.MaterializeNextCategoryCorrection(ctx)
	if err != nil {
		t.Fatalf("materialize correction: %v", err)
	}
	if !processed {
		t.Fatal("expected correction event to be processed")
	}
	if len(receiptRepo.hints) != 1 || receiptRepo.hints[0].CanonicalName != "Exponenta cocktail" {
		t.Fatalf("expected LLM canonical hint, got %+v", receiptRepo.hints)
	}
	if receiptRepo.correctionEvents[0].ProcessedAt == nil {
		t.Fatalf("expected event processed, got %+v", receiptRepo.correctionEvents[0])
	}
}

func TestMaterializeNextCategoryCorrectionFallsBackOnLowConfidence(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newMaterializerServiceWithEvent(&fakeHintNormalizer{
		result: &NormalizeCategoryCorrectionResult{
			Action:        NormalizeActionCreateNew,
			CanonicalName: "Protein cocktail",
			Confidence:    0.3,
		},
	})

	processed, err := service.MaterializeNextCategoryCorrection(ctx)
	if err != nil {
		t.Fatalf("materialize correction: %v", err)
	}
	if !processed {
		t.Fatal("expected correction event to be processed")
	}
	if len(receiptRepo.hints) != 1 || receiptRepo.hints[0].CanonicalName != "EXPONENTA 30g" {
		t.Fatalf("expected deterministic fallback hint, got %+v", receiptRepo.hints)
	}
}

func TestMaterializeNextCategoryCorrectionRetriesNormalizerError(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newMaterializerServiceWithEvent(&fakeHintNormalizer{err: errors.New("boom")})

	processed, err := service.MaterializeNextCategoryCorrection(ctx)
	if err != nil {
		t.Fatalf("materialize correction: %v", err)
	}
	if !processed {
		t.Fatal("expected correction event to be acquired")
	}
	event := receiptRepo.correctionEvents[0]
	if event.ProcessedAt != nil {
		t.Fatalf("expected event to remain unprocessed, got %+v", event)
	}
	if event.NextMaterializeAttemptAt == nil || event.MaterializeErrorCode == nil || *event.MaterializeErrorCode != "normalizer_failed" {
		t.Fatalf("expected retry metadata, got %+v", event)
	}
	if len(receiptRepo.hints) != 0 {
		t.Fatalf("expected no hint before retry succeeds, got %+v", receiptRepo.hints)
	}
}

func TestMaterializeNextCategoryCorrectionFallsBackAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newMaterializerServiceWithEvent(&fakeHintNormalizer{err: errors.New("boom")})
	receiptRepo.correctionEvents[0].MaterializeAttemptCount = defaultHintMaterializerMaxAttempts - 1

	processed, err := service.MaterializeNextCategoryCorrection(ctx)
	if err != nil {
		t.Fatalf("materialize correction: %v", err)
	}
	if !processed {
		t.Fatal("expected correction event to be acquired")
	}
	if len(receiptRepo.hints) != 1 || receiptRepo.hints[0].CanonicalName != "EXPONENTA 30g" {
		t.Fatalf("expected deterministic fallback after max attempts, got %+v", receiptRepo.hints)
	}
	if receiptRepo.correctionEvents[0].ProcessedAt == nil {
		t.Fatalf("expected event processed after fallback, got %+v", receiptRepo.correctionEvents[0])
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

func TestProcessNextPassesTopAllowedHintsToParser(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	fileStore := newMemoryReceiptFileStore()
	parser := &captureParser{}
	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	receiptRepo.hints = []FamilyHint{
		{
			ID:              "hint-allowed",
			FamilyID:        testFamilyID,
			CanonicalName:   "Exponenta cocktail",
			FinalCategoryID: testSportID,
			TimesConfirmed:  3,
			LastConfirmedAt: now,
		},
		{
			ID:              "hint-filtered",
			FamilyID:        testFamilyID,
			CanonicalName:   "Taxi",
			FinalCategoryID: "77777777-7777-7777-7777-777777777777",
			TimesConfirmed:  10,
			LastConfirmedAt: now,
		},
	}
	service := NewServiceWithOptions(receiptRepo, parser, fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
			{ID: testSportID, FamilyID: testFamilyID, Name: "Sport"},
		},
	}, fakeExpenseBatchCreator{}, ServiceOptions{
		FileStore:     fileStore,
		WorkerEnabled: false,
		WorkerID:      "test-worker",
	})

	job, err := service.CreateParse(ctx, CreateParseInput{
		FamilyID:            testFamilyID,
		UserID:              testUserID,
		CategoryMode:        CategoryModeSelected,
		SelectedCategoryIDs: []string{testCategoryID, testSportID},
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
	if parser.input.File.FileName == "" || job.ID == "" {
		t.Fatalf("expected parser to receive input for job %s", job.ID)
	}
	if len(parser.input.Corrections) != 1 {
		t.Fatalf("expected one allowed correction hint, got %+v", parser.input.Corrections)
	}
	hint := parser.input.Corrections[0]
	if hint.CanonicalName != "Exponenta cocktail" || hint.CategoryID != testSportID || hint.CategoryName != "Sport" || hint.TimesConfirmed != 3 {
		t.Fatalf("unexpected correction hint %+v", hint)
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

type captureParser struct {
	input ParseReceiptInput
}

func (p *captureParser) ParseReceipt(_ context.Context, input ParseReceiptInput) (*ParsedReceipt, error) {
	p.input = input
	categoryID := input.Categories[0].ID
	total := 10.0
	return &ParsedReceipt{
		Currency:    "BYN",
		Provider:    "fake",
		Model:       "fake",
		RawResponse: []byte(`{"fake":true}`),
		Items: []ParsedItem{
			{
				RawName:    "Receipt item",
				LineTotal:  total,
				CategoryID: &categoryID,
			},
		},
	}, nil
}

type fakeHintNormalizer struct {
	result *NormalizeCategoryCorrectionResult
	err    error
	input  NormalizeCategoryCorrectionInput
}

func (n *fakeHintNormalizer) NormalizeCategoryCorrection(_ context.Context, input NormalizeCategoryCorrectionInput) (*NormalizeCategoryCorrectionResult, error) {
	n.input = input
	if n.err != nil {
		return nil, n.err
	}
	return n.result, nil
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

func newReadyApproveServiceWithItem(item Item) (*fakeReceiptRepo, *Service) {
	receiptRepo := newFakeReceiptRepo()
	expenseRepo := newFakeReceiptExpenseRepo()
	receiptRepo.expenseRepo = expenseRepo
	receiptRepo.jobs[testJobID] = &Job{
		ID:       testJobID,
		FamilyID: testFamilyID,
		UserID:   testUserID,
		Status:   StatusReady,
	}
	receiptRepo.items[testJobID] = []Item{item}
	receiptRepo.drafts[testJobID] = []DraftExpense{
		{
			ID:         testDraftID,
			JobID:      testJobID,
			Title:      "Draft",
			Amount:     10,
			Currency:   "BYN",
			CategoryID: stringValue(item.FinalCategoryID),
			Warnings:   []byte("[]"),
		},
	}
	service := NewServiceWithOptions(receiptRepo, nil, nil, fakeExpenseBatchCreator{}, ServiceOptions{WorkerEnabled: false})
	return receiptRepo, service
}

func newMaterializerServiceWithEvent(normalizer HintNormalizer) (*fakeReceiptRepo, *Service) {
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	receiptRepo.correctionEvents = []CategoryCorrectionEvent{
		{
			ID:                 "event-1",
			FamilyID:           testFamilyID,
			UserID:             testUserID,
			ReceiptParseJobID:  testJobID,
			ReceiptParseItemID: "item-1",
			SourceItemText:     "Exponenta raw",
			NormalizedItemText: "EXPONENTA 30g",
			LLMCategoryID:      stringPtr(testCategoryID),
			FinalCategoryID:    testSportID,
			CreatedAt:          time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC),
		},
	}
	service := NewServiceWithOptions(receiptRepo, nil, fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
			{ID: testSportID, FamilyID: testFamilyID, Name: "Sport"},
		},
	}, fakeExpenseBatchCreator{}, ServiceOptions{
		HintNormalizer: normalizer,
		WorkerEnabled:  false,
	})
	return receiptRepo, service
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
	jobs             map[string]*Job
	files            map[string][]File
	items            map[string][]Item
	drafts           map[string][]DraftExpense
	correctionEvents []CategoryCorrectionEvent
	hints            []FamilyHint
	hintExamples     []FamilyHintExample
	expenseRepo      *fakeReceiptExpenseRepo
	failUpdateJob    bool
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
	if r.expenseRepo != nil {
		tx.expenseRepo = r.expenseRepo.clone()
	} else {
		tx.expenseRepo = newFakeReceiptExpenseRepo()
	}
	if err := fn(tx, tx.expenseRepo); err != nil {
		return err
	}
	r.jobs = tx.jobs
	r.files = tx.files
	r.items = tx.items
	r.drafts = tx.drafts
	r.correctionEvents = tx.correctionEvents
	r.hints = tx.hints
	r.hintExamples = tx.hintExamples
	if r.expenseRepo != nil {
		r.expenseRepo.expenses = tx.expenseRepo.expenses
		r.expenseRepo.expenseCategories = tx.expenseRepo.expenseCategories
	}
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
	clone.correctionEvents = append([]CategoryCorrectionEvent{}, r.correctionEvents...)
	clone.hints = append([]FamilyHint{}, r.hints...)
	clone.hintExamples = append([]FamilyHintExample{}, r.hintExamples...)
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

func (r *fakeReceiptRepo) CreateCategoryCorrectionEvent(_ context.Context, event *CategoryCorrectionEvent) error {
	for _, existing := range r.correctionEvents {
		if existing.ReceiptParseItemID == event.ReceiptParseItemID {
			return ErrReceiptParseInvalidStatus
		}
	}
	r.correctionEvents = append(r.correctionEvents, *event)
	return nil
}

func (r *fakeReceiptRepo) AcquireUnprocessedCategoryCorrectionEvent(_ context.Context, workerID string, now time.Time) (*CategoryCorrectionEvent, error) {
	for index := range r.correctionEvents {
		event := &r.correctionEvents[index]
		if event.ProcessedAt != nil {
			continue
		}
		if event.LockedAt != nil {
			continue
		}
		if event.NextMaterializeAttemptAt != nil && event.NextMaterializeAttemptAt.After(now) {
			continue
		}
		event.MaterializeAttemptCount++
		event.LastMaterializeAttemptAt = &now
		event.LockedAt = &now
		event.LockedBy = &workerID
		event.MaterializeErrorCode = nil
		event.MaterializeErrorMessage = nil
		eventCopy := *event
		return &eventCopy, nil
	}
	return nil, nil
}

func (r *fakeReceiptRepo) RequeueStaleCategoryCorrections(_ context.Context, staleBefore time.Time) (int64, error) {
	var count int64
	for index := range r.correctionEvents {
		event := &r.correctionEvents[index]
		if event.ProcessedAt != nil || event.LockedAt == nil || !event.LockedAt.Before(staleBefore) {
			continue
		}
		event.LockedAt = nil
		event.LockedBy = nil
		count++
	}
	return count, nil
}

func (r *fakeReceiptRepo) MarkCategoryCorrectionEventProcessed(_ context.Context, eventID string, processedAt time.Time) error {
	for index := range r.correctionEvents {
		if r.correctionEvents[index].ID != eventID {
			continue
		}
		r.correctionEvents[index].ProcessedAt = &processedAt
		r.correctionEvents[index].LockedAt = nil
		r.correctionEvents[index].LockedBy = nil
		r.correctionEvents[index].NextMaterializeAttemptAt = nil
		r.correctionEvents[index].MaterializeErrorCode = nil
		r.correctionEvents[index].MaterializeErrorMessage = nil
		return nil
	}
	return ErrReceiptParseInvalidStatus
}

func (r *fakeReceiptRepo) ReleaseCategoryCorrectionEventWithError(_ context.Context, eventID, code, message string, nextAttemptAt *time.Time) error {
	for index := range r.correctionEvents {
		if r.correctionEvents[index].ID != eventID {
			continue
		}
		r.correctionEvents[index].LockedAt = nil
		r.correctionEvents[index].LockedBy = nil
		r.correctionEvents[index].NextMaterializeAttemptAt = nextAttemptAt
		r.correctionEvents[index].MaterializeErrorCode = &code
		r.correctionEvents[index].MaterializeErrorMessage = &message
		return nil
	}
	return ErrReceiptParseInvalidStatus
}

func (r *fakeReceiptRepo) UpsertFamilyHint(_ context.Context, input UpsertFamilyHintInput) (*FamilyHint, error) {
	for index := range r.hints {
		hint := &r.hints[index]
		if hint.FamilyID == input.FamilyID && hint.CanonicalName == input.CanonicalName && hint.FinalCategoryID == input.FinalCategoryID {
			hint.TimesConfirmed++
			hint.LastConfirmedAt = input.ConfirmedAt
			hint.UpdatedAt = input.ConfirmedAt
			hintCopy := *hint
			return &hintCopy, nil
		}
	}
	hint := FamilyHint{
		ID:              input.ID,
		FamilyID:        input.FamilyID,
		CanonicalName:   input.CanonicalName,
		FinalCategoryID: input.FinalCategoryID,
		TimesConfirmed:  1,
		LastConfirmedAt: input.ConfirmedAt,
		CreatedAt:       input.ConfirmedAt,
		UpdatedAt:       input.ConfirmedAt,
	}
	r.hints = append(r.hints, hint)
	hintCopy := hint
	return &hintCopy, nil
}

func (r *fakeReceiptRepo) CreateFamilyHintExample(_ context.Context, example *FamilyHintExample) error {
	r.hintExamples = append(r.hintExamples, *example)
	return nil
}

func (r *fakeReceiptRepo) ListFamilyHints(_ context.Context, familyID string, categoryIDs []string, limit int) ([]FamilyHint, error) {
	if limit <= 0 || len(categoryIDs) == 0 {
		return []FamilyHint{}, nil
	}
	allowed := make(map[string]struct{}, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		allowed[categoryID] = struct{}{}
	}
	result := make([]FamilyHint, 0, len(r.hints))
	for _, hint := range r.hints {
		if hint.FamilyID != familyID {
			continue
		}
		if _, ok := allowed[hint.FinalCategoryID]; !ok {
			continue
		}
		result = append(result, hint)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].TimesConfirmed != result[j].TimesConfirmed {
			return result[i].TimesConfirmed > result[j].TimesConfirmed
		}
		return result[i].LastConfirmedAt.After(result[j].LastConfirmedAt)
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return append([]FamilyHint{}, result...), nil
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
