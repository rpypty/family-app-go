# Receipt Category Correction Memory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Store family-specific receipt item category corrections, normalize them asynchronously into canonical hints, and inject top correction hints into future receipt parser prompts.

**Architecture:** Add raw correction events plus derived hint tables, write only raw correction events inside approve, and materialize canonical hints in a separate DB-backed worker. The materializer uses a `HintNormalizer` interface with OpenAI `gpt-5.4-nano` as the configured LLM implementation and deterministic fallback when the model is unavailable, invalid, or low-confidence.

**Tech Stack:** Go, GORM, PostgreSQL migrations, existing receipt domain service tests, existing OpenAI parser HTTP tests.

---

## File Structure

- Create `migrations/0024_create_receipt_parse_correction_tables.sql`: correction events, canonical hints, hint examples, indexes, uniqueness constraints.
- Modify `internal/domain/receipts/model.go`: add correction/hint structs, upsert input, and `ParseReceiptInput.Corrections`.
- Modify `internal/domain/receipts/repository.go`: add repository methods for correction events, hint upsert, examples, and top hint listing.
- Modify `internal/repository/postgres/receipts/postgres.go`: implement new repository methods using GORM and PostgreSQL upsert.
- Modify `internal/domain/receipts/service.go`: add correction collection/persistence during approve and hint loading during parse processing.
- Modify `internal/domain/receipts/service_test.go`: extend fake repository and add focused service tests.
- Modify `internal/repository/http/receipts/openai.go`: append the family hint prompt block when corrections are present.
- Create `internal/repository/http/receipts/hint_normalizer_openai.go`: normalize raw correction events with structured OpenAI output.
- Modify `internal/config/config.go`: add hint normalizer config fields and default model `gpt-5.4-nano`.
- Modify `internal/app/receipt_parser.go`: build the OpenAI hint normalizer separately from the image parser.
- Modify `internal/app/app.go`: pass the hint normalizer into the receipt service.
- Modify `internal/repository/http/receipts/openai_test.go`: verify prompt behavior and unchanged category schema.

## Async Materializer Addendum

The implementation has been extended from the initial MVP plan:

- `ApproveParse` creates `CategoryCorrectionEvent` only.
- `runHintMaterializer` acquires unprocessed correction events from the database.
- The materializer calls `HintNormalizer.NormalizeCategoryCorrection`.
- Valid high-confidence `match_existing` results increment the matched hint.
- Valid high-confidence `create_new` results create or increment a canonical hint by `(family_id, canonical_name, final_category_id)`.
- Invalid, low-confidence, disabled, or repeatedly failing LLM results use deterministic fallback: `normalized_item_text`, then `source_item_text`.
- Hint examples are created by the materializer, not by approve.
- Events are marked `processed_at` only after hint/example writes succeed.

## Task 1: Migration And Domain Contracts

**Files:**
- Create: `migrations/0024_create_receipt_parse_correction_tables.sql`
- Modify: `internal/domain/receipts/model.go`
- Modify: `internal/domain/receipts/repository.go`

- [ ] **Step 1: Add the migration**

Create `migrations/0024_create_receipt_parse_correction_tables.sql`:

```sql
CREATE TABLE IF NOT EXISTS receipt_parse_category_correction_events (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  user_id UUID NOT NULL,
  receipt_parse_job_id UUID NOT NULL REFERENCES receipt_parse_jobs(id) ON DELETE CASCADE,
  receipt_parse_item_id UUID NOT NULL REFERENCES receipt_parse_items(id) ON DELETE CASCADE,
  source_item_text TEXT NOT NULL,
  normalized_item_text TEXT NOT NULL,
  llm_category_id UUID NULL,
  final_category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  processed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(receipt_parse_item_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_correction_events_family_created
  ON receipt_parse_category_correction_events(family_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_correction_events_unprocessed
  ON receipt_parse_category_correction_events(processed_at)
  WHERE processed_at IS NULL;

CREATE TABLE IF NOT EXISTS receipt_parse_family_hints (
  id UUID PRIMARY KEY,
  family_id UUID NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  canonical_name TEXT NOT NULL,
  final_category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
  times_confirmed INTEGER NOT NULL DEFAULT 1,
  last_confirmed_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(family_id, canonical_name, final_category_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_family_hints_family_rank
  ON receipt_parse_family_hints(family_id, times_confirmed DESC, last_confirmed_at DESC);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_family_hints_family_category
  ON receipt_parse_family_hints(family_id, final_category_id);

CREATE TABLE IF NOT EXISTS receipt_parse_family_hint_examples (
  id UUID PRIMARY KEY,
  hint_id UUID NOT NULL REFERENCES receipt_parse_family_hints(id) ON DELETE CASCADE,
  correction_event_id UUID NOT NULL REFERENCES receipt_parse_category_correction_events(id) ON DELETE CASCADE,
  source_item_text TEXT NOT NULL,
  normalized_item_text TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(hint_id, correction_event_id)
);

CREATE INDEX IF NOT EXISTS idx_receipt_parse_family_hint_examples_hint_created
  ON receipt_parse_family_hint_examples(hint_id, created_at DESC);
```

- [ ] **Step 2: Add domain structs**

In `internal/domain/receipts/model.go`, add these types after `DraftExpense`:

```go
type CategoryCorrectionEvent struct {
	ID                 string     `gorm:"type:uuid;primaryKey"`
	FamilyID           string     `gorm:"type:uuid;index;not null"`
	UserID             string     `gorm:"type:uuid;not null"`
	ReceiptParseJobID  string     `gorm:"type:uuid;not null"`
	ReceiptParseItemID string     `gorm:"type:uuid;not null"`
	SourceItemText     string     `gorm:"not null"`
	NormalizedItemText string     `gorm:"not null"`
	LLMCategoryID      *string    `gorm:"type:uuid"`
	FinalCategoryID    string     `gorm:"type:uuid;not null"`
	ProcessedAt        *time.Time `gorm:"type:timestamptz"`
	CreatedAt          time.Time  `gorm:"autoCreateTime"`
}

func (CategoryCorrectionEvent) TableName() string {
	return "receipt_parse_category_correction_events"
}

type FamilyHint struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	FamilyID        string    `gorm:"type:uuid;index;not null"`
	CanonicalName   string    `gorm:"not null"`
	FinalCategoryID string    `gorm:"type:uuid;not null"`
	TimesConfirmed  int       `gorm:"not null"`
	LastConfirmedAt time.Time `gorm:"not null"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

func (FamilyHint) TableName() string {
	return "receipt_parse_family_hints"
}

type FamilyHintExample struct {
	ID                 string    `gorm:"type:uuid;primaryKey"`
	HintID             string    `gorm:"type:uuid;index;not null"`
	CorrectionEventID  string    `gorm:"type:uuid;not null"`
	SourceItemText     string    `gorm:"not null"`
	NormalizedItemText string    `gorm:"not null"`
	CreatedAt          time.Time `gorm:"autoCreateTime"`
}

func (FamilyHintExample) TableName() string {
	return "receipt_parse_family_hint_examples"
}

type UpsertFamilyHintInput struct {
	ID              string
	FamilyID        string
	CanonicalName   string
	FinalCategoryID string
	ConfirmedAt     time.Time
}

type CorrectionHint struct {
	CanonicalName  string
	CategoryID     string
	CategoryName   string
	TimesConfirmed int
}
```

- [ ] **Step 3: Extend parser input**

Update `ParseReceiptInput` in `internal/domain/receipts/model.go`:

```go
type ParseReceiptInput struct {
	File        UploadedFile
	Categories  []Category
	Date        *time.Time
	Currency    string
	Corrections []CorrectionHint
}
```

- [ ] **Step 4: Extend repository interface**

Add methods to `internal/domain/receipts/repository.go`:

```go
CreateCategoryCorrectionEvent(ctx context.Context, event *CategoryCorrectionEvent) error
UpsertFamilyHint(ctx context.Context, input UpsertFamilyHintInput) (*FamilyHint, error)
CreateFamilyHintExample(ctx context.Context, example *FamilyHintExample) error
ListFamilyHints(ctx context.Context, familyID string, categoryIDs []string, limit int) ([]FamilyHint, error)
```

- [ ] **Step 5: Run compile check**

Run:

```bash
go test ./internal/domain/receipts
```

Expected: FAIL because concrete repositories and fake repositories do not yet implement the extended interface.

## Task 2: Postgres Repository Methods

**Files:**
- Modify: `internal/repository/postgres/receipts/postgres.go`

- [ ] **Step 1: Implement event/example creation**

Add methods to `internal/repository/postgres/receipts/postgres.go`:

```go
func (r *PostgresRepository) CreateCategoryCorrectionEvent(ctx context.Context, event *receiptsdomain.CategoryCorrectionEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *PostgresRepository) CreateFamilyHintExample(ctx context.Context, example *receiptsdomain.FamilyHintExample) error {
	return r.db.WithContext(ctx).Create(example).Error
}
```

- [ ] **Step 2: Implement hint upsert**

Add `UpsertFamilyHint` using `clause.OnConflict`:

```go
func (r *PostgresRepository) UpsertFamilyHint(ctx context.Context, input receiptsdomain.UpsertFamilyHintInput) (*receiptsdomain.FamilyHint, error) {
	hint := receiptsdomain.FamilyHint{
		ID:              input.ID,
		FamilyID:        input.FamilyID,
		CanonicalName:   input.CanonicalName,
		FinalCategoryID: input.FinalCategoryID,
		TimesConfirmed:  1,
		LastConfirmedAt: input.ConfirmedAt,
		CreatedAt:       input.ConfirmedAt,
		UpdatedAt:       input.ConfirmedAt,
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "family_id"},
				{Name: "canonical_name"},
				{Name: "final_category_id"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"times_confirmed":  gorm.Expr("receipt_parse_family_hints.times_confirmed + 1"),
				"last_confirmed_at": input.ConfirmedAt,
				"updated_at":        input.ConfirmedAt,
			}),
		}).
		Create(&hint).Error
	if err != nil {
		return nil, err
	}

	var persisted receiptsdomain.FamilyHint
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND canonical_name = ? AND final_category_id = ?", input.FamilyID, input.CanonicalName, input.FinalCategoryID).
		First(&persisted).Error; err != nil {
		return nil, err
	}
	return &persisted, nil
}
```

- [ ] **Step 3: Implement ranked hint listing**

Add:

```go
func (r *PostgresRepository) ListFamilyHints(ctx context.Context, familyID string, categoryIDs []string, limit int) ([]receiptsdomain.FamilyHint, error) {
	if len(categoryIDs) == 0 || limit <= 0 {
		return []receiptsdomain.FamilyHint{}, nil
	}
	var hints []receiptsdomain.FamilyHint
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND final_category_id IN ?", familyID, categoryIDs).
		Order("times_confirmed DESC").
		Order("last_confirmed_at DESC").
		Limit(limit).
		Find(&hints).Error; err != nil {
		return nil, err
	}
	return hints, nil
}
```

- [ ] **Step 4: Run repository package compile check**

Run:

```bash
go test ./internal/repository/postgres/receipts
```

Expected: PASS or no test files after compilation succeeds.

## Task 3: Service Correction Persistence

**Files:**
- Modify: `internal/domain/receipts/service.go`
- Modify: `internal/domain/receipts/service_test.go`

- [ ] **Step 1: Add failing approve tests**

Add these tests to `internal/domain/receipts/service_test.go` near existing approve tests:

```go
func TestApproveParseCreatesCorrectionEventForChangedCategory(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newReadyApproveServiceWithItem(Item{
		ID:                 "item-1",
		JobID:              testJobID,
		RawName:            "Exponenta cocktail",
		NormalizedName:     stringPtr("Exponenta cocktail"),
		LineTotal:          10,
		FinalLineTotal:     floatPtr(10),
		LLMCategoryID:      stringPtr(testCategoryID),
		FinalCategoryID:    stringPtr(testSecondCategoryID),
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
			CategoryIDs: []string{testSecondCategoryID},
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
	if event.LLMCategoryID == nil || *event.LLMCategoryID != testCategoryID || event.FinalCategoryID != testSecondCategoryID {
		t.Fatalf("unexpected event categories %+v", event)
	}
	if len(receiptRepo.hints) != 1 || receiptRepo.hints[0].TimesConfirmed != 1 {
		t.Fatalf("expected one new hint, got %+v", receiptRepo.hints)
	}
	if len(receiptRepo.hintExamples) != 1 {
		t.Fatalf("expected one hint example, got %d", len(receiptRepo.hintExamples))
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
		FinalCategoryID: stringPtr(testSecondCategoryID),
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
			CategoryIDs: []string{testSecondCategoryID},
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
```

- [ ] **Step 2: Add a second category test constant**

Add next to `testCategoryID`:

```go
testSecondCategoryID = "66666666-6666-6666-6666-666666666666"
```

- [ ] **Step 3: Extend the fake repository state**

Add fields to `fakeReceiptRepo`:

```go
correctionEvents []CategoryCorrectionEvent
hints            []FamilyHint
hintExamples     []FamilyHintExample
```

Copy those slices in `clone()` and commit them in `Transaction()`:

```go
r.correctionEvents = tx.correctionEvents
r.hints = tx.hints
r.hintExamples = tx.hintExamples
```

- [ ] **Step 4: Add fake repository methods**

Implement:

```go
func (r *fakeReceiptRepo) CreateCategoryCorrectionEvent(_ context.Context, event *CategoryCorrectionEvent) error {
	for _, existing := range r.correctionEvents {
		if existing.ReceiptParseItemID == event.ReceiptParseItemID {
			return ErrReceiptParseInvalidStatus
		}
	}
	r.correctionEvents = append(r.correctionEvents, *event)
	return nil
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
```

Add `sort` to the test imports.

- [ ] **Step 5: Add test helper**

Add:

```go
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
	receiptRepo.drafts[testJobID] = []DraftExpense{{
		ID:         testDraftID,
		JobID:      testJobID,
		Title:      "Draft",
		Amount:     10,
		Currency:   "BYN",
		CategoryID: stringValue(item.FinalCategoryID),
		Warnings:   []byte("[]"),
	}}
	service := NewServiceWithOptions(receiptRepo, nil, nil, fakeExpenseBatchCreator{}, ServiceOptions{WorkerEnabled: false})
	return receiptRepo, service
}
```

- [ ] **Step 6: Run tests and confirm failures**

Run:

```bash
go test ./internal/domain/receipts
```

Expected: FAIL because service does not create correction memory yet.

- [ ] **Step 7: Implement correction persistence**

In `internal/domain/receipts/service.go`, add helpers:

```go
func (s *Service) persistCategoryCorrections(ctx context.Context, repo Repository, familyID, userID, jobID string, items []Item, now time.Time) error {
	for _, item := range items {
		if !shouldPersistCategoryCorrection(item) {
			continue
		}
		eventID, err := newUUID()
		if err != nil {
			return err
		}
		hintID, err := newUUID()
		if err != nil {
			return err
		}
		exampleID, err := newUUID()
		if err != nil {
			return err
		}

		normalized := canonicalItemText(item)
		event := &CategoryCorrectionEvent{
			ID:                 eventID,
			FamilyID:           familyID,
			UserID:             userID,
			ReceiptParseJobID:  jobID,
			ReceiptParseItemID: item.ID,
			SourceItemText:     strings.TrimSpace(item.RawName),
			NormalizedItemText: normalized,
			LLMCategoryID:      item.LLMCategoryID,
			FinalCategoryID:    *item.FinalCategoryID,
			CreatedAt:          now,
		}
		if err := repo.CreateCategoryCorrectionEvent(ctx, event); err != nil {
			return err
		}

		hint, err := repo.UpsertFamilyHint(ctx, UpsertFamilyHintInput{
			ID:              hintID,
			FamilyID:        familyID,
			CanonicalName:   normalized,
			FinalCategoryID: *item.FinalCategoryID,
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
		if err := repo.CreateFamilyHintExample(ctx, example); err != nil {
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
```

Call `persistCategoryCorrections` inside the approve transaction after draft updates and before status change:

```go
now := time.Now().UTC()
if err := s.persistCategoryCorrections(ctx, receiptTx, input.FamilyID, input.UserID, input.JobID, items, now); err != nil {
	return err
}
currentJob.Status = StatusApproved
currentJob.ApprovedAt = &now
currentJob.UpdatedAt = now
return receiptTx.UpdateJob(ctx, currentJob)
```

- [ ] **Step 8: Run focused service tests**

Run:

```bash
go test ./internal/domain/receipts
```

Expected: PASS.

## Task 4: Repeated Hint Counts And Parse-Time Hint Loading

**Files:**
- Modify: `internal/domain/receipts/service.go`
- Modify: `internal/domain/receipts/service_test.go`

- [ ] **Step 1: Add repeated correction test**

Add:

```go
func TestApproveParseRepeatedCorrectionIncrementsHintCount(t *testing.T) {
	ctx := context.Background()
	receiptRepo, service := newReadyApproveServiceWithItem(Item{
		ID:              "item-1",
		JobID:           testJobID,
		RawName:         "Exponenta cocktail",
		NormalizedName:  stringPtr("Exponenta cocktail"),
		LineTotal:       10,
		FinalLineTotal:  floatPtr(10),
		LLMCategoryID:   stringPtr(testCategoryID),
		FinalCategoryID: stringPtr(testSecondCategoryID),
	})
	receiptRepo.hints = []FamilyHint{{
		ID:              "hint-1",
		FamilyID:        testFamilyID,
		CanonicalName:   "Exponenta cocktail",
		FinalCategoryID: testSecondCategoryID,
		TimesConfirmed:  1,
		LastConfirmedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
	}}
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
			CategoryIDs: []string{testSecondCategoryID},
		}},
	})
	if err != nil {
		t.Fatalf("approve parse: %v", err)
	}
	if len(receiptRepo.hints) != 1 || receiptRepo.hints[0].TimesConfirmed != 2 {
		t.Fatalf("expected existing hint count to increment, got %+v", receiptRepo.hints)
	}
}
```

- [ ] **Step 2: Add capturing parser**

Add to `service_test.go`:

```go
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
		Items: []ParsedItem{{
			RawName:    "Receipt item",
			LineTotal:  total,
			CategoryID: &categoryID,
		}},
	}, nil
}
```

- [ ] **Step 3: Add parser input hint test**

Add:

```go
func TestProcessNextPassesTopAllowedHintsToParser(t *testing.T) {
	ctx := context.Background()
	receiptRepo := newFakeReceiptRepo()
	receiptRepo.expenseRepo = newFakeReceiptExpenseRepo()
	fileStore := newMemoryReceiptFileStore()
	parser := &captureParser{}
	now := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	receiptRepo.hints = []FamilyHint{
		{ID: "hint-allowed", FamilyID: testFamilyID, CanonicalName: "Exponenta cocktail", FinalCategoryID: testSecondCategoryID, TimesConfirmed: 3, LastConfirmedAt: now},
		{ID: "hint-filtered", FamilyID: testFamilyID, CanonicalName: "Taxi", FinalCategoryID: "77777777-7777-7777-7777-777777777777", TimesConfirmed: 10, LastConfirmedAt: now},
	}
	service := NewServiceWithOptions(receiptRepo, parser, fakeCategoryProvider{
		categories: []expensesdomain.Category{
			{ID: testCategoryID, FamilyID: testFamilyID, Name: "Products"},
			{ID: testSecondCategoryID, FamilyID: testFamilyID, Name: "Sport"},
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
		SelectedCategoryIDs: []string{testCategoryID, testSecondCategoryID},
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
	if hint.CanonicalName != "Exponenta cocktail" || hint.CategoryID != testSecondCategoryID || hint.CategoryName != "Sport" || hint.TimesConfirmed != 3 {
		t.Fatalf("unexpected correction hint %+v", hint)
	}
}
```

- [ ] **Step 4: Run test and confirm hint injection fails**

Run:

```bash
go test ./internal/domain/receipts
```

Expected: FAIL because `processJob` does not load hints yet.

- [ ] **Step 5: Implement hint loading**

In `processJob`, after `categories` is resolved and before loading the file, add:

```go
corrections, err := s.correctionHintsForCategories(ctx, job.FamilyID, categories, 20)
if err != nil {
	s.markFailed(ctx, job, "internal_error", "failed to load receipt parser hints")
	return
}
```

Pass `corrections` into `ParseReceiptInput`:

```go
parsed, parseErr := s.parser.ParseReceipt(ctx, ParseReceiptInput{
	File:        file,
	Categories:  categories,
	Date:        job.RequestedDate,
	Currency:    stringValue(job.RequestedCurrency),
	Corrections: corrections,
})
```

Add helper:

```go
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
```

- [ ] **Step 6: Run focused service tests**

Run:

```bash
go test ./internal/domain/receipts
```

Expected: PASS.

## Task 5: OpenAI Prompt Hint Block

**Files:**
- Modify: `internal/repository/http/receipts/openai.go`
- Modify: `internal/repository/http/receipts/openai_test.go`

- [ ] **Step 1: Add failing prompt test**

Add to `openai_test.go`:

```go
func TestOpenAIParserBuildRequestIncludesFamilyCorrectionHints(t *testing.T) {
	parser, err := NewOpenAIParser(OpenAIParserConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	raw, err := parser.buildRequest(receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{
			{ID: "cat-products", Name: "Products"},
			{ID: "cat-sport", Name: "Sport"},
		},
		Currency: "BYN",
		Corrections: []receiptsdomain.CorrectionHint{
			{CanonicalName: "Exponenta cocktail", CategoryID: "cat-sport", CategoryName: "Sport", TimesConfirmed: 2},
		},
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	var request openAIRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	userText := request.Input[1].Content[0].Text
	if !strings.Contains(userText, "Family-specific category hints:") {
		t.Fatalf("missing hints block: %q", userText)
	}
	if !strings.Contains(userText, `"Exponenta cocktail" -> "Sport"`) {
		t.Fatalf("missing hint mapping: %q", userText)
	}
	if !strings.Contains(userText, "Use these as soft hints only") {
		t.Fatalf("missing soft hint instruction: %q", userText)
	}
}
```

- [ ] **Step 2: Add no-hints prompt test**

Add:

```go
func TestOpenAIParserBuildRequestOmitsFamilyCorrectionHintsWhenEmpty(t *testing.T) {
	parser, err := NewOpenAIParser(OpenAIParserConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	raw, err := parser.buildRequest(receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{{ID: "cat-1", Name: "Products"}},
		Currency:   "BYN",
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	var request openAIRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	userText := request.Input[1].Content[0].Text
	if strings.Contains(userText, "Family-specific category hints:") {
		t.Fatalf("expected no hints block: %q", userText)
	}
}
```

- [ ] **Step 3: Run OpenAI parser tests and confirm failure**

Run:

```bash
go test ./internal/repository/http/receipts
```

Expected: FAIL because the prompt does not include hints yet.

- [ ] **Step 4: Implement prompt block**

In `openai.go`, add helper:

```go
func buildCorrectionHintsBlock(corrections []receiptsdomain.CorrectionHint) string {
	if len(corrections) == 0 {
		return ""
	}
	lines := []string{
		"Family-specific category hints:",
	}
	for _, correction := range corrections {
		name := strings.TrimSpace(correction.CanonicalName)
		categoryName := strings.TrimSpace(correction.CategoryName)
		if name == "" || categoryName == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %q -> %q", name, categoryName))
	}
	if len(lines) == 1 {
		return ""
	}
	lines = append(lines,
		"",
		"Use these as soft hints only.",
		"Do not treat them as strict rules.",
		"If the current receipt item is clearly different, ignore the hint.",
	)
	return strings.Join(lines, "\n")
}
```

Change user prompt construction from a single `fmt.Sprintf` to a slice:

```go
userPromptParts := []string{
	fmt.Sprintf(
		"Parse this receipt image.\nAllowed categories:\n%s\nRequested date: %s\nRequested currency: %s\nKeep item names in the original receipt language.\nUse the category IDs exactly as listed.",
		strings.Join(categoryLines, "\n"),
		emptyAsUnknown(requestedDate),
		emptyAsUnknown(requestedCurrency),
	),
}
if hintsBlock := buildCorrectionHintsBlock(input.Corrections); hintsBlock != "" {
	userPromptParts = append(userPromptParts, hintsBlock)
}
userPrompt := strings.Join(userPromptParts, "\n\n")
```

- [ ] **Step 5: Run OpenAI parser tests**

Run:

```bash
go test ./internal/repository/http/receipts
```

Expected: PASS.

## Task 6: Formatting And Full Verification

**Files:**
- All modified Go files

- [ ] **Step 1: Format Go files**

Run:

```bash
gofmt -w internal/domain/receipts/model.go internal/domain/receipts/repository.go internal/domain/receipts/service.go internal/domain/receipts/service_test.go internal/repository/postgres/receipts/postgres.go internal/repository/http/receipts/openai.go internal/repository/http/receipts/openai_test.go
```

Expected: no output.

- [ ] **Step 2: Run focused receipt tests**

Run:

```bash
go test ./internal/domain/receipts ./internal/repository/http/receipts ./internal/repository/postgres/receipts
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Inspect git diff**

Run:

```bash
git diff --stat
git diff --check
git status --short
```

Expected:

- `git diff --check` has no output.
- Only intended files are modified.

## Self-Review Checklist

- Spec coverage: migration, domain models, repository methods, approve corrections, hint injection, OpenAI prompt, and tests are each covered by a task.
- Type consistency: `CorrectionHint`, `UpsertFamilyHintInput`, `FamilyHint.FinalCategoryID`, and `ParseReceiptInput.Corrections` names are used consistently across tasks.
- Scope: the plan implements the MVP plus future materializer contract direction, without adding the async materializer itself.
- Documentation command constraint: commands in this plan omit the local shell proxy prefix because project instructions say not to write it in documentation.
