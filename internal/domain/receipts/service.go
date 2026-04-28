package receipts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
)

const (
	defaultWakeQueueSize                    = 1
	defaultPollInterval                     = time.Second
	defaultStaleAfter                       = 15 * time.Minute
	defaultWorkerID                         = "receipt-parser"
	defaultHintMaterializerWorkerID         = "receipt-hint-materializer"
	defaultHintMaterializerMaxAttempts      = 3
	defaultHintMaterializerRetryDelay       = time.Minute
	defaultHintMaterializerConfidenceCutoff = 0.7
	defaultFileStoreRoot                    = "data/receipt-parses"
)

type Parser interface {
	ParseReceipt(ctx context.Context, input ParseReceiptInput) (*ParsedReceipt, error)
}

type HintNormalizer interface {
	NormalizeCategoryCorrection(ctx context.Context, input NormalizeCategoryCorrectionInput) (*NormalizeCategoryCorrectionResult, error)
}

type CategoryProvider interface {
	ListCategories(ctx context.Context, familyID string) ([]expensesdomain.Category, error)
}

type ExpenseBatchCreator interface {
	CreateExpensesBatch(ctx context.Context, inputs []expensesdomain.CreateExpenseInput) ([]expensesdomain.ExpenseWithCategories, error)
	CreateExpensesBatchWithRepository(ctx context.Context, repo expensesdomain.Repository, inputs []expensesdomain.CreateExpenseInput) ([]expensesdomain.ExpenseWithCategories, error)
}

type Service struct {
	repo         Repository
	parser       Parser
	normalizer   HintNormalizer
	categories   CategoryProvider
	expenses     ExpenseBatchCreator
	fileStore    FileStore
	workerID     string
	hintWorkerID string
	pollInterval time.Duration
	staleAfter   time.Duration
	wake         chan struct{}
	hintWake     chan struct{}
}

type ServiceOptions struct {
	FileStore      FileStore
	WorkerEnabled  bool
	WorkerID       string
	HintNormalizer HintNormalizer
	HintWorkerID   string
	PollInterval   time.Duration
	StaleAfter     time.Duration
}

func NewService(repo Repository, parser Parser, categories CategoryProvider, expenses ExpenseBatchCreator) *Service {
	return NewServiceWithOptions(repo, parser, categories, expenses, ServiceOptions{
		FileStore:     NewLocalFileStore(defaultFileStoreRoot),
		WorkerEnabled: true,
	})
}

func NewServiceWithOptions(repo Repository, parser Parser, categories CategoryProvider, expenses ExpenseBatchCreator, options ServiceOptions) *Service {
	fileStore := options.FileStore
	if fileStore == nil {
		fileStore = NewLocalFileStore(defaultFileStoreRoot)
	}
	workerID := strings.TrimSpace(options.WorkerID)
	if workerID == "" {
		workerID = defaultWorkerID
	}
	hintWorkerID := strings.TrimSpace(options.HintWorkerID)
	if hintWorkerID == "" {
		hintWorkerID = defaultHintMaterializerWorkerID
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	staleAfter := options.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultStaleAfter
	}

	service := &Service{
		repo:         repo,
		parser:       parser,
		normalizer:   options.HintNormalizer,
		categories:   categories,
		expenses:     expenses,
		fileStore:    fileStore,
		workerID:     workerID,
		hintWorkerID: hintWorkerID,
		pollInterval: pollInterval,
		staleAfter:   staleAfter,
		wake:         make(chan struct{}, defaultWakeQueueSize),
		hintWake:     make(chan struct{}, defaultWakeQueueSize),
	}
	if options.WorkerEnabled {
		go service.runWorker()
		go service.runHintMaterializer()
	}
	return service
}

func (s *Service) CreateParse(ctx context.Context, input CreateParseInput) (*Job, error) {
	if s.parser == nil {
		return nil, ErrReceiptParserDisabled
	}
	if err := validateUploadedFile(input.File); err != nil {
		return nil, err
	}

	categories, selectedIDs, err := s.resolveCategories(ctx, input.FamilyID, input.CategoryMode, input.SelectedCategoryIDs)
	if err != nil {
		return nil, err
	}
	if len(categories) == 0 {
		return nil, ErrCategorySelectionRequired
	}

	active, err := s.repo.CountActiveJobs(ctx, input.FamilyID)
	if err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, ErrActiveReceiptParseExists
	}

	jobID, err := newUUID()
	if err != nil {
		return nil, err
	}
	fileID, err := newUUID()
	if err != nil {
		return nil, err
	}
	storageKey, err := s.fileStore.Save(ctx, jobID, fileID, input.File)
	if err != nil {
		return nil, err
	}
	selectedJSON, err := json.Marshal(selectedIDs)
	if err != nil {
		return nil, err
	}

	var requestedCurrency *string
	if trimmed := strings.TrimSpace(input.RequestedCurrency); trimmed != "" {
		upper := strings.ToUpper(trimmed)
		requestedCurrency = &upper
	}

	job := &Job{
		ID:                  jobID,
		FamilyID:            input.FamilyID,
		UserID:              input.UserID,
		Status:              StatusQueued,
		CategoryMode:        input.CategoryMode,
		SelectedCategoryIDs: selectedJSON,
		RequestedDate:       input.RequestedDate,
		RequestedCurrency:   requestedCurrency,
	}
	file := &File{
		ID:          fileID,
		JobID:       jobID,
		Ordinal:     0,
		FileName:    input.File.FileName,
		ContentType: input.File.ContentType,
		SizeBytes:   input.File.SizeBytes,
		StorageKey:  &storageKey,
		SHA256:      stringPtr(input.File.SHA256),
	}

	err = s.repo.Transaction(ctx, func(tx Repository, _ expensesdomain.Repository) error {
		if err := tx.CreateJob(ctx, job); err != nil {
			return err
		}
		return tx.CreateFile(ctx, file)
	})
	if err != nil {
		return nil, err
	}

	s.wakeWorker()

	return job, nil
}

func (s *Service) GetActiveParse(ctx context.Context, familyID string) (*Job, error) {
	return s.repo.GetActiveJob(ctx, familyID)
}

func (s *Service) GetParse(ctx context.Context, familyID, jobID string) (*JobWithDrafts, error) {
	job, err := s.repo.GetJobByID(ctx, familyID, jobID)
	if err != nil {
		return nil, err
	}
	drafts, err := s.repo.ListDraftExpenses(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItemsByJobID(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	return &JobWithDrafts{Job: *job, DraftExpenses: drafts, Items: items}, nil
}

func (s *Service) CancelParse(ctx context.Context, familyID, jobID string) (*Job, error) {
	job, err := s.repo.GetJobByID(ctx, familyID, jobID)
	if err != nil {
		return nil, err
	}
	if !isActiveStatus(job.Status) {
		return nil, ErrReceiptParseInvalidStatus
	}
	now := time.Now().UTC()
	job.Status = StatusCancelled
	job.CancelledAt = &now
	job.UpdatedAt = now
	if err := s.repo.UpdateJob(ctx, job); err != nil {
		return nil, err
	}
	s.cleanupStoredFiles(ctx, job.ID)
	return job, nil
}

func (s *Service) ApproveParse(ctx context.Context, input ApproveInput) ([]expensesdomain.ExpenseWithCategories, error) {
	job, err := s.repo.GetJobByID(ctx, input.FamilyID, input.JobID)
	if err != nil {
		return nil, err
	}
	if job.Status != StatusReady {
		return nil, ErrReceiptParseInvalidStatus
	}
	if len(input.Expenses) == 0 {
		return nil, ErrReceiptParseEmpty
	}

	drafts, err := s.repo.ListDraftExpenses(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListItemsByJobID(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	if hasUnresolvedItems(items) {
		return nil, ErrReceiptParseUnresolvedItems
	}
	draftByID := make(map[string]DraftExpense, len(drafts))
	for _, draft := range drafts {
		draftByID[draft.ID] = draft
	}

	expenseInputs := make([]expensesdomain.CreateExpenseInput, 0, len(input.Expenses))
	updatedDrafts := make([]DraftExpense, 0, len(input.Expenses))
	for _, item := range input.Expenses {
		draft, ok := draftByID[item.DraftID]
		if !ok {
			return nil, ErrReceiptParseInvalidStatus
		}
		title := strings.TrimSpace(item.Title)
		currency := strings.ToUpper(strings.TrimSpace(item.Currency))
		if title == "" || item.Amount <= 0 || currency == "" || len(item.CategoryIDs) == 0 {
			return nil, ErrReceiptParseInvalidStatus
		}

		finalCategoryID := item.CategoryIDs[0]
		draft.FinalTitle = &title
		draft.FinalAmount = &item.Amount
		draft.FinalCategoryID = &finalCategoryID
		draft.EditedByUser = draft.Title != title || draft.Amount != item.Amount || draft.CategoryID != finalCategoryID
		draft.UpdatedAt = time.Now().UTC()
		updatedDrafts = append(updatedDrafts, draft)

		expenseInputs = append(expenseInputs, expensesdomain.CreateExpenseInput{
			FamilyID:     input.FamilyID,
			UserID:       input.UserID,
			Date:         item.Date,
			Amount:       item.Amount,
			Currency:     currency,
			BaseCurrency: input.BaseCurrency,
			Title:        title,
			CategoryIDs:  item.CategoryIDs,
		})
	}

	var created []expensesdomain.ExpenseWithCategories
	err = s.repo.Transaction(ctx, func(receiptTx Repository, expenseTx expensesdomain.Repository) error {
		currentJob, err := receiptTx.GetJobByID(ctx, input.FamilyID, input.JobID)
		if err != nil {
			return err
		}
		if currentJob.Status != StatusReady {
			return ErrReceiptParseInvalidStatus
		}

		created, err = s.expenses.CreateExpensesBatchWithRepository(ctx, expenseTx, expenseInputs)
		if err != nil {
			if errors.Is(err, expensesdomain.ErrCategoryNotFound) {
				return ErrCategoryNotFound
			}
			return err
		}

		for _, draft := range updatedDrafts {
			draftCopy := draft
			if err := receiptTx.UpdateDraftExpense(ctx, &draftCopy); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		if err := s.persistCategoryCorrections(ctx, receiptTx, input.FamilyID, input.UserID, currentJob.ID, items, now); err != nil {
			return err
		}
		currentJob.Status = StatusApproved
		currentJob.ApprovedAt = &now
		currentJob.UpdatedAt = now
		return receiptTx.UpdateJob(ctx, currentJob)
	})
	if err != nil {
		return nil, err
	}

	s.wakeHintMaterializer()
	s.cleanupStoredFiles(ctx, job.ID)

	return created, nil
}

func (s *Service) UpdateItems(ctx context.Context, input UpdateItemsInput) (*JobWithDrafts, error) {
	job, err := s.repo.GetJobByID(ctx, input.FamilyID, input.JobID)
	if err != nil {
		return nil, err
	}
	if job.Status != StatusReady {
		return nil, ErrReceiptParseInvalidStatus
	}

	items, err := s.repo.ListItemsByJobID(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	categories, err := s.categoriesForJob(ctx, job)
	if err != nil {
		return nil, err
	}
	categoryNames := make(map[string]string, len(categories))
	for _, category := range categories {
		categoryNames[category.ID] = category.Name
	}

	updateByID := make(map[string]ReviewItemInput, len(input.Items))
	for _, update := range input.Items {
		itemID := strings.TrimSpace(update.ItemID)
		if itemID == "" {
			return nil, ErrReceiptParseInvalidStatus
		}
		updateByID[itemID] = update
	}

	for index := range items {
		update, ok := updateByID[items[index].ID]
		if !ok {
			continue
		}
		if update.Amount != nil {
			if *update.Amount <= 0 {
				return nil, ErrReceiptParseInvalidStatus
			}
			value := roundMoney(*update.Amount)
			items[index].FinalLineTotal = &value
		}
		if update.CategoryID != nil {
			categoryID := strings.TrimSpace(*update.CategoryID)
			if categoryID == "" {
				items[index].FinalCategoryID = nil
			} else {
				if _, ok := categoryNames[categoryID]; !ok {
					return nil, ErrCategoryNotFound
				}
				items[index].FinalCategoryID = &categoryID
			}
		}
		items[index].EditedByUser = true
	}

	drafts, err := buildDraftsFromItems(job.ID, items, categoryNames, stringValue(job.Currency), stringValue(job.RequestedCurrency))
	if err != nil {
		return nil, err
	}

	err = s.repo.Transaction(ctx, func(tx Repository, _ expensesdomain.Repository) error {
		for _, item := range items {
			itemCopy := item
			if err := tx.UpdateItem(ctx, &itemCopy); err != nil {
				return err
			}
		}
		if err := tx.ReplaceDraftExpenses(ctx, job.ID, drafts); err != nil {
			return err
		}
		job.UpdatedAt = time.Now().UTC()
		return tx.UpdateJob(ctx, job)
	})
	if err != nil {
		return nil, err
	}

	return s.GetParse(ctx, input.FamilyID, input.JobID)
}

func (s *Service) runWorker() {
	ctx := context.Background()
	_ = s.RecoverStaleProcessing(ctx)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		_, _ = s.ProcessNext(ctx)
		select {
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) runHintMaterializer() {
	ctx := context.Background()
	_ = s.RecoverStaleCategoryCorrections(ctx)
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		_, _ = s.MaterializeNextCategoryCorrection(ctx)
		select {
		case <-s.hintWake:
		case <-ticker.C:
		}
	}
}

func (s *Service) wakeWorker() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) wakeHintMaterializer() {
	select {
	case s.hintWake <- struct{}{}:
	default:
	}
}

func (s *Service) RecoverStaleProcessing(ctx context.Context) error {
	_, err := s.repo.RequeueStaleProcessing(ctx, time.Now().UTC().Add(-s.staleAfter))
	return err
}

func (s *Service) RecoverStaleCategoryCorrections(ctx context.Context) error {
	_, err := s.repo.RequeueStaleCategoryCorrections(ctx, time.Now().UTC().Add(-s.staleAfter))
	return err
}

func (s *Service) ProcessNext(ctx context.Context) (bool, error) {
	job, err := s.repo.AcquireQueuedJob(ctx, s.workerID, time.Now().UTC())
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}
	s.processJob(ctx, job)
	return true, nil
}

func (s *Service) MaterializeNextCategoryCorrection(ctx context.Context) (bool, error) {
	now := time.Now().UTC()
	event, err := s.repo.AcquireUnprocessedCategoryCorrectionEvent(ctx, s.hintWorkerID, now)
	if err != nil {
		return false, err
	}
	if event == nil {
		return false, nil
	}
	if err := s.materializeCategoryCorrection(ctx, event, now); err != nil {
		return true, nil
	}
	return true, nil
}

func (s *Service) processJob(ctx context.Context, job *Job) {
	categories, err := s.categoriesForJob(ctx, job)
	if err != nil {
		s.markFailed(ctx, job, "category_not_found", "receipt parse categories are not available")
		return
	}
	if len(categories) == 0 {
		s.markFailed(ctx, job, "category_selection_required", "category selection is required")
		return
	}

	corrections, err := s.correctionHintsForCategories(ctx, job.FamilyID, categories, 20)
	if err != nil {
		s.markFailed(ctx, job, "internal_error", "failed to load receipt parser hints")
		return
	}

	file, err := s.loadJobFile(ctx, job.ID)
	if err != nil {
		s.markFailed(ctx, job, "receipt_file_unavailable", "receipt file is unavailable")
		return
	}

	parsed, parseErr := s.parser.ParseReceipt(ctx, ParseReceiptInput{
		File:        file,
		Categories:  categories,
		Date:        job.RequestedDate,
		Currency:    stringValue(job.RequestedCurrency),
		Corrections: corrections,
	})
	if parseErr != nil {
		if errors.Is(parseErr, ErrLLMInvalidResponse) {
			s.markFailed(ctx, job, "llm_invalid_response", "invalid receipt parser response")
			return
		}
		s.markFailed(ctx, job, "llm_request_failed", "failed to parse receipt")
		return
	}

	items, drafts, err := s.normalizeParsed(job.ID, parsed, categories)
	if err != nil {
		s.markFailed(ctx, job, "llm_invalid_response", "invalid receipt parser response")
		return
	}
	if len(items) == 0 {
		s.markFailed(ctx, job, "receipt_parse_empty", "receipt parse produced no draft expenses")
		return
	}

	now := time.Now().UTC()
	total := sumItems(items)

	err = s.repo.Transaction(ctx, func(tx Repository, _ expensesdomain.Repository) error {
		currentJob, err := tx.GetJobByID(ctx, job.FamilyID, job.ID)
		if err != nil {
			return err
		}
		if currentJob.Status != StatusProcessing {
			return nil
		}

		if err := tx.ReplaceItems(ctx, job.ID, items); err != nil {
			return err
		}
		if err := tx.ReplaceDraftExpenses(ctx, job.ID, drafts); err != nil {
			return err
		}

		currentJob.Status = StatusReady
		currentJob.MerchantName = parsed.MerchantName
		currentJob.PurchasedAt = parsed.PurchasedAt
		if parsed.Currency != "" {
			currentJob.Currency = &parsed.Currency
		}
		currentJob.DetectedTotal = parsed.DetectedTotal
		currentJob.ItemsTotal = &total
		currentJob.Provider = stringPtr(parsed.Provider)
		currentJob.Model = stringPtr(parsed.Model)
		currentJob.RawLLMResponse = parsed.RawResponse
		currentJob.CompletedAt = &now
		currentJob.UpdatedAt = now
		currentJob.LockedAt = nil
		currentJob.LockedBy = nil
		currentJob.ErrorCode = nil
		currentJob.ErrorMessage = nil
		return tx.UpdateJob(ctx, currentJob)
	})
	if err != nil {
		s.markFailed(ctx, job, "internal_error", "failed to persist receipt parse")
	}
}

func (s *Service) categoriesForJob(ctx context.Context, job *Job) ([]Category, error) {
	var selected []string
	if len(job.SelectedCategoryIDs) > 0 {
		if err := json.Unmarshal(job.SelectedCategoryIDs, &selected); err != nil {
			return nil, err
		}
	}
	mode := job.CategoryMode
	if len(selected) > 0 {
		mode = CategoryModeSelected
	}
	categories, _, err := s.resolveCategories(ctx, job.FamilyID, mode, selected)
	return categories, err
}

func (s *Service) loadJobFile(ctx context.Context, jobID string) (UploadedFile, error) {
	files, err := s.repo.ListFilesByJobID(ctx, jobID)
	if err != nil {
		return UploadedFile{}, err
	}
	if len(files) == 0 || files[0].StorageKey == nil || strings.TrimSpace(*files[0].StorageKey) == "" {
		return UploadedFile{}, ErrInvalidReceiptFile
	}
	data, err := s.fileStore.Load(ctx, *files[0].StorageKey)
	if err != nil {
		return UploadedFile{}, err
	}
	file := files[0]
	return UploadedFile{
		FileName:    file.FileName,
		ContentType: file.ContentType,
		SizeBytes:   int64(len(data)),
		SHA256:      stringValue(file.SHA256),
		Data:        data,
	}, nil
}

func (s *Service) cleanupStoredFiles(ctx context.Context, jobID string) {
	files, err := s.repo.ListFilesByJobID(ctx, jobID)
	if err != nil {
		return
	}
	for _, file := range files {
		if file.StorageKey == nil || strings.TrimSpace(*file.StorageKey) == "" {
			continue
		}
		_ = s.fileStore.Delete(ctx, *file.StorageKey)
	}
}

func (s *Service) markFailed(ctx context.Context, job *Job, code, message string) {
	latest, err := s.repo.GetJobByID(ctx, job.FamilyID, job.ID)
	if err == nil {
		if latest.Status == StatusCancelled || latest.Status == StatusApproved {
			return
		}
		job = latest
	}

	now := time.Now().UTC()
	job.Status = StatusFailed
	job.ErrorCode = &code
	job.ErrorMessage = &message
	job.CompletedAt = &now
	job.UpdatedAt = now
	job.LockedAt = nil
	job.LockedBy = nil
	_ = s.repo.UpdateJob(ctx, job)
}

func (s *Service) resolveCategories(ctx context.Context, familyID string, mode CategoryMode, selected []string) ([]Category, []string, error) {
	all, err := s.categories.ListCategories(ctx, familyID)
	if err != nil {
		return nil, nil, err
	}
	allByID := make(map[string]expensesdomain.Category, len(all))
	for _, category := range all {
		allByID[category.ID] = category
	}

	if mode == CategoryModeAll {
		result := make([]Category, 0, len(all))
		ids := make([]string, 0, len(all))
		for _, category := range all {
			result = append(result, Category{ID: category.ID, Name: category.Name})
			ids = append(ids, category.ID)
		}
		return result, ids, nil
	}

	ids := normalizeIDs(selected)
	if len(ids) == 0 {
		return nil, nil, ErrCategorySelectionRequired
	}
	result := make([]Category, 0, len(ids))
	for _, id := range ids {
		category, ok := allByID[id]
		if !ok {
			return nil, nil, ErrCategoryNotFound
		}
		result = append(result, Category{ID: category.ID, Name: category.Name})
	}
	return result, ids, nil
}

func (s *Service) normalizeParsed(jobID string, parsed *ParsedReceipt, categories []Category) ([]Item, []DraftExpense, error) {
	if parsed == nil || len(parsed.Items) == 0 {
		return nil, nil, ErrReceiptParseEmpty
	}
	categoryNames := make(map[string]string, len(categories))
	for _, category := range categories {
		categoryNames[category.ID] = category.Name
	}

	items := make([]Item, 0, len(parsed.Items))
	aggregates := make(map[string]*DraftExpense)
	for index, parsedItem := range parsed.Items {
		name := strings.TrimSpace(parsedItem.RawName)
		if name == "" || parsedItem.LineTotal <= 0 {
			continue
		}

		itemID, err := newUUID()
		if err != nil {
			return nil, nil, err
		}
		lineTotal := roundMoney(parsedItem.LineTotal)
		var categoryID *string
		if parsedItem.CategoryID != nil {
			trimmed := strings.TrimSpace(*parsedItem.CategoryID)
			if _, ok := categoryNames[trimmed]; ok {
				categoryID = &trimmed
			}
		}
		items = append(items, Item{
			ID:                    itemID,
			JobID:                 jobID,
			LineIndex:             index,
			RawName:               name,
			NormalizedName:        parsedItem.NormalizedName,
			Quantity:              parsedItem.Quantity,
			UnitPrice:             parsedItem.UnitPrice,
			LineTotal:             lineTotal,
			LLMCategoryID:         categoryID,
			LLMCategoryConfidence: parsedItem.CategoryConfidence,
			FinalCategoryID:       categoryID,
			FinalLineTotal:        &lineTotal,
		})

		if categoryID == nil {
			continue
		}

		categoryName := categoryNames[*categoryID]
		aggregate := aggregates[*categoryID]
		if aggregate == nil {
			draftID, err := newUUID()
			if err != nil {
				return nil, nil, err
			}
			warnings, _ := json.Marshal([]string{})
			currency := strings.TrimSpace(parsed.Currency)
			if currency == "" {
				currency = "BYN"
			}
			aggregate = &DraftExpense{
				ID:         draftID,
				JobID:      jobID,
				Title:      categoryName,
				Currency:   strings.ToUpper(currency),
				CategoryID: *categoryID,
				Warnings:   warnings,
			}
			aggregates[*categoryID] = aggregate
		}
		aggregate.Amount = roundMoney(aggregate.Amount + lineTotal)
		aggregate.Confidence = minConfidence(aggregate.Confidence, parsedItem.CategoryConfidence)
	}

	drafts := make([]DraftExpense, 0, len(aggregates))
	for _, draft := range aggregates {
		drafts = append(drafts, *draft)
	}
	return items, drafts, nil
}

func buildDraftsFromItems(jobID string, items []Item, categoryNames map[string]string, preferredCurrencies ...string) ([]DraftExpense, error) {
	aggregates := make(map[string]*DraftExpense)
	currency := "BYN"
	for _, candidate := range preferredCurrencies {
		trimmed := strings.ToUpper(strings.TrimSpace(candidate))
		if trimmed != "" {
			currency = trimmed
			break
		}
	}

	for _, item := range items {
		if item.IsDeleted || item.FinalLineTotal == nil || item.FinalCategoryID == nil {
			continue
		}
		categoryID := strings.TrimSpace(*item.FinalCategoryID)
		categoryName, ok := categoryNames[categoryID]
		if !ok {
			continue
		}

		aggregate := aggregates[categoryID]
		if aggregate == nil {
			draftID, err := newUUID()
			if err != nil {
				return nil, err
			}
			warnings, _ := json.Marshal([]string{})
			aggregate = &DraftExpense{
				ID:         draftID,
				JobID:      jobID,
				Title:      categoryName,
				Currency:   currency,
				CategoryID: categoryID,
				Warnings:   warnings,
			}
			aggregates[categoryID] = aggregate
		}
		aggregate.Amount = roundMoney(aggregate.Amount + *item.FinalLineTotal)
		aggregate.Confidence = minConfidence(aggregate.Confidence, item.LLMCategoryConfidence)
	}

	drafts := make([]DraftExpense, 0, len(aggregates))
	for _, draft := range aggregates {
		drafts = append(drafts, *draft)
	}
	return drafts, nil
}

func (s *Service) persistCategoryCorrections(ctx context.Context, repo Repository, familyID, userID, jobID string, items []Item, now time.Time) error {
	for _, item := range items {
		if !shouldPersistCategoryCorrection(item) {
			continue
		}
		normalized := canonicalItemText(item)
		sourceText := strings.TrimSpace(item.RawName)
		if normalized == "" || sourceText == "" {
			continue
		}

		eventID, err := newUUID()
		if err != nil {
			return err
		}

		event := &CategoryCorrectionEvent{
			ID:                 eventID,
			FamilyID:           familyID,
			UserID:             userID,
			ReceiptParseJobID:  jobID,
			ReceiptParseItemID: item.ID,
			SourceItemText:     sourceText,
			NormalizedItemText: normalized,
			LLMCategoryID:      item.LLMCategoryID,
			FinalCategoryID:    strings.TrimSpace(*item.FinalCategoryID),
			CreatedAt:          now,
		}
		if err := repo.CreateCategoryCorrectionEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func shouldPersistCategoryCorrection(item Item) bool {
	if item.IsDeleted || item.FinalCategoryID == nil || strings.TrimSpace(*item.FinalCategoryID) == "" {
		return false
	}
	if item.LLMCategoryID == nil || strings.TrimSpace(*item.LLMCategoryID) == "" {
		return true
	}
	return strings.TrimSpace(*item.LLMCategoryID) != strings.TrimSpace(*item.FinalCategoryID)
}

func canonicalItemText(item Item) string {
	if item.NormalizedName != nil {
		if normalized := strings.TrimSpace(*item.NormalizedName); normalized != "" {
			return normalized
		}
	}
	return strings.TrimSpace(item.RawName)
}

func (s *Service) materializeCategoryCorrection(ctx context.Context, event *CategoryCorrectionEvent, now time.Time) error {
	finalCategory, llmCategory, existingHints, err := s.materializerContext(ctx, event)
	if err != nil {
		return s.releaseMaterializerError(ctx, event, "context_unavailable", err.Error(), now)
	}

	canonicalName := deterministicCanonicalName(*event)
	if s.normalizer != nil {
		result, err := s.normalizer.NormalizeCategoryCorrection(ctx, NormalizeCategoryCorrectionInput{
			Event:            *event,
			FinalCategory:    finalCategory,
			LLMCategory:      llmCategory,
			ExistingHints:    existingHints,
			ConfidenceCutoff: defaultHintMaterializerConfidenceCutoff,
		})
		if err != nil {
			if event.MaterializeAttemptCount < defaultHintMaterializerMaxAttempts {
				return s.releaseMaterializerError(ctx, event, "normalizer_failed", err.Error(), now)
			}
		} else if normalized, ok := validNormalizerCanonicalName(result, existingHints, defaultHintMaterializerConfidenceCutoff); ok {
			canonicalName = normalized
		}
	}

	return s.persistMaterializedHint(ctx, event, canonicalName, now)
}

func (s *Service) materializerContext(ctx context.Context, event *CategoryCorrectionEvent) (Category, *Category, []FamilyHint, error) {
	var finalCategory Category
	var llmCategory *Category

	categories, err := s.categories.ListCategories(ctx, event.FamilyID)
	if err != nil {
		return Category{}, nil, nil, err
	}
	for _, category := range categories {
		domainCategory := Category{ID: category.ID, Name: category.Name}
		if category.ID == event.FinalCategoryID {
			finalCategory = domainCategory
		}
		if event.LLMCategoryID != nil && category.ID == *event.LLMCategoryID {
			copyCategory := domainCategory
			llmCategory = &copyCategory
		}
	}
	if strings.TrimSpace(finalCategory.ID) == "" {
		finalCategory = Category{ID: event.FinalCategoryID, Name: event.FinalCategoryID}
	}

	existingHints, err := s.repo.ListFamilyHints(ctx, event.FamilyID, []string{event.FinalCategoryID}, 50)
	if err != nil {
		return Category{}, nil, nil, err
	}
	return finalCategory, llmCategory, existingHints, nil
}

func validNormalizerCanonicalName(result *NormalizeCategoryCorrectionResult, existingHints []FamilyHint, confidenceCutoff float64) (string, bool) {
	if result == nil || result.Confidence < confidenceCutoff {
		return "", false
	}
	switch result.Action {
	case NormalizeActionMatchExisting:
		if result.HintID == nil || strings.TrimSpace(*result.HintID) == "" {
			return "", false
		}
		for _, hint := range existingHints {
			if hint.ID == *result.HintID {
				if canonicalName := strings.TrimSpace(hint.CanonicalName); canonicalName != "" {
					return canonicalName, true
				}
			}
		}
		return "", false
	case NormalizeActionCreateNew:
		canonicalName := strings.TrimSpace(result.CanonicalName)
		return canonicalName, canonicalName != ""
	default:
		return "", false
	}
}

func (s *Service) persistMaterializedHint(ctx context.Context, event *CategoryCorrectionEvent, canonicalName string, now time.Time) error {
	canonicalName = strings.TrimSpace(canonicalName)
	if canonicalName == "" {
		canonicalName = deterministicCanonicalName(*event)
	}
	hintID, err := newUUID()
	if err != nil {
		return err
	}
	exampleID, err := newUUID()
	if err != nil {
		return err
	}

	return s.repo.Transaction(ctx, func(tx Repository, _ expensesdomain.Repository) error {
		hint, err := tx.UpsertFamilyHint(ctx, UpsertFamilyHintInput{
			ID:              hintID,
			FamilyID:        event.FamilyID,
			CanonicalName:   canonicalName,
			FinalCategoryID: event.FinalCategoryID,
			ConfirmedAt:     now,
		})
		if err != nil {
			return err
		}
		example := &FamilyHintExample{
			ID:                 exampleID,
			HintID:             hint.ID,
			CorrectionEventID:  event.ID,
			SourceItemText:     event.SourceItemText,
			NormalizedItemText: event.NormalizedItemText,
			CreatedAt:          now,
		}
		if err := tx.CreateFamilyHintExample(ctx, example); err != nil {
			return err
		}
		return tx.MarkCategoryCorrectionEventProcessed(ctx, event.ID, now)
	})
}

func (s *Service) releaseMaterializerError(ctx context.Context, event *CategoryCorrectionEvent, code, message string, now time.Time) error {
	if event.MaterializeAttemptCount >= defaultHintMaterializerMaxAttempts {
		return s.persistMaterializedHint(ctx, event, deterministicCanonicalName(*event), now)
	}
	nextAttemptAt := now.Add(defaultHintMaterializerRetryDelay)
	return s.repo.ReleaseCategoryCorrectionEventWithError(ctx, event.ID, code, truncateErrorMessage(message), &nextAttemptAt)
}

func deterministicCanonicalName(event CategoryCorrectionEvent) string {
	if normalized := strings.TrimSpace(event.NormalizedItemText); normalized != "" {
		return normalized
	}
	return strings.TrimSpace(event.SourceItemText)
}

func truncateErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 500 {
		return message
	}
	return message[:500]
}

func (s *Service) correctionHintsForCategories(ctx context.Context, familyID string, categories []Category, limit int) ([]CorrectionHint, error) {
	if len(categories) == 0 || limit <= 0 {
		return []CorrectionHint{}, nil
	}
	categoryNames := make(map[string]string, len(categories))
	categoryIDs := make([]string, 0, len(categories))
	for _, category := range categories {
		categoryIDs = append(categoryIDs, category.ID)
		categoryNames[category.ID] = category.Name
	}

	hints, err := s.repo.ListFamilyHints(ctx, familyID, categoryIDs, limit)
	if err != nil {
		return nil, err
	}
	result := make([]CorrectionHint, 0, len(hints))
	for _, hint := range hints {
		categoryName, ok := categoryNames[hint.FinalCategoryID]
		if !ok {
			continue
		}
		result = append(result, CorrectionHint{
			CanonicalName:  hint.CanonicalName,
			CategoryID:     hint.FinalCategoryID,
			CategoryName:   categoryName,
			TimesConfirmed: hint.TimesConfirmed,
		})
	}
	return result, nil
}

func hasUnresolvedItems(items []Item) bool {
	for _, item := range items {
		if item.IsDeleted {
			continue
		}
		if item.FinalCategoryID == nil || strings.TrimSpace(*item.FinalCategoryID) == "" {
			return true
		}
	}
	return false
}

func validateUploadedFile(file UploadedFile) error {
	if file.SizeBytes <= 0 || len(file.Data) == 0 {
		return ErrInvalidReceiptFile
	}
	if file.SizeBytes > 8*1024*1024 {
		return ErrReceiptFileTooLarge
	}
	switch file.ContentType {
	case "image/jpeg", "image/png", "image/webp":
		return nil
	default:
		return ErrInvalidReceiptFile
	}
}

func normalizeIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func isActiveStatus(status ParseStatus) bool {
	switch status {
	case StatusQueued, StatusProcessing, StatusReady, StatusFailed:
		return true
	default:
		return false
	}
}

func sumItems(items []Item) float64 {
	var total float64
	for _, item := range items {
		total += item.LineTotal
	}
	return roundMoney(total)
}

func minConfidence(current *float64, next *float64) *float64 {
	if next == nil {
		return current
	}
	if current == nil || *next < *current {
		value := *next
		return &value
	}
	return current
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(b[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
