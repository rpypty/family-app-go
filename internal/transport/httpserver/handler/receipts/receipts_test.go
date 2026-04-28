package receipts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	receiptsdomain "family-app-go/internal/domain/receipts"
	"family-app-go/internal/transport/httpserver/middleware"
	"family-app-go/pkg/logger"
	"github.com/go-chi/chi/v5"
)

const (
	handlerFamilyID   = "11111111-1111-1111-1111-111111111111"
	handlerUserID     = "22222222-2222-2222-2222-222222222222"
	handlerCategoryID = "33333333-3333-3333-3333-333333333333"
	handlerJobID      = "44444444-4444-4444-4444-444444444444"
	handlerDraftID    = "55555555-5555-5555-5555-555555555555"
)

func TestGetActiveParseReturnsNullWhenNoActiveJob(t *testing.T) {
	h := newTestHandlers(newHandlerReceiptRepo())
	req := authenticatedRequest(http.MethodGet, "/api/receipt-parses/active", nil)
	rec := httptest.NewRecorder()

	h.GetActiveParse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body activeParseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Item != nil {
		t.Fatalf("expected null item, got %#v", body.Item)
	}
}

func TestCreateParseReturnsConflictWhenActiveJobExists(t *testing.T) {
	repo := newHandlerReceiptRepo()
	repo.jobs[handlerJobID] = &receiptsdomain.Job{
		ID:       handlerJobID,
		FamilyID: handlerFamilyID,
		UserID:   handlerUserID,
		Status:   receiptsdomain.StatusReady,
	}
	h := newTestHandlers(repo)
	body, contentType := multipartReceiptBody(t)
	req := authenticatedRequest(http.MethodPost, "/api/receipt-parses", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()

	h.CreateParse(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"]["code"] != "active_receipt_parse_exists" {
		t.Fatalf("unexpected error code: %#v", payload)
	}
}

func TestGetParseReturnsReadyDrafts(t *testing.T) {
	repo := newHandlerReceiptRepo()
	total := 10.0
	requestedDate := time.Date(2026, time.April, 27, 0, 0, 0, 0, time.UTC)
	repo.jobs[handlerJobID] = &receiptsdomain.Job{
		ID:            handlerJobID,
		FamilyID:      handlerFamilyID,
		UserID:        handlerUserID,
		Status:        receiptsdomain.StatusReady,
		RequestedDate: &requestedDate,
		Currency:      testStringPtr("BYN"),
		DetectedTotal: &total,
		ItemsTotal:    &total,
	}
	repo.drafts[handlerJobID] = []receiptsdomain.DraftExpense{
		{
			ID:         handlerDraftID,
			JobID:      handlerJobID,
			Title:      "Products",
			Amount:     total,
			Currency:   "BYN",
			CategoryID: handlerCategoryID,
			Warnings:   []byte("[]"),
		},
	}
	repo.items[handlerJobID] = []receiptsdomain.Item{
		{
			ID:              "item-1",
			JobID:           handlerJobID,
			RawName:         "Milk",
			LineTotal:       total,
			FinalLineTotal:  &total,
			LLMCategoryID:   testStringPtr(handlerCategoryID),
			FinalCategoryID: testStringPtr(handlerCategoryID),
		},
		{
			ID:             "item-2",
			JobID:          handlerJobID,
			RawName:        "Unknown",
			LineTotal:      2,
			FinalLineTotal: floatPtr(2),
		},
	}
	h := newTestHandlers(repo)
	req := authenticatedRequest(http.MethodGet, "/api/receipt-parses/"+handlerJobID, nil)
	req = withURLParam(req, "id", handlerJobID)
	rec := httptest.NewRecorder()

	h.GetParse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body receiptParseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != receiptsdomain.StatusReady {
		t.Fatalf("expected ready status, got %s", body.Status)
	}
	if len(body.DraftExpenses) != 1 || body.DraftExpenses[0].ID != handlerDraftID {
		t.Fatalf("unexpected drafts: %#v", body.DraftExpenses)
	}
	if body.Receipt.RequestedDate == nil || *body.Receipt.RequestedDate != "2026-04-27" {
		t.Fatalf("unexpected requested_date: %#v", body.Receipt.RequestedDate)
	}
	if len(body.Items) != 1 || len(body.UnresolvedItems) != 1 {
		t.Fatalf("unexpected item split: resolved=%d unresolved=%d", len(body.Items), len(body.UnresolvedItems))
	}
}

func TestGetParseReturnsNullRequestedDateWhenNotSet(t *testing.T) {
	repo := newHandlerReceiptRepo()
	total := 10.0
	repo.jobs[handlerJobID] = &receiptsdomain.Job{
		ID:            handlerJobID,
		FamilyID:      handlerFamilyID,
		UserID:        handlerUserID,
		Status:        receiptsdomain.StatusReady,
		Currency:      testStringPtr("BYN"),
		DetectedTotal: &total,
		ItemsTotal:    &total,
	}
	h := newTestHandlers(repo)
	req := authenticatedRequest(http.MethodGet, "/api/receipt-parses/"+handlerJobID, nil)
	req = withURLParam(req, "id", handlerJobID)
	rec := httptest.NewRecorder()

	h.GetParse(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body receiptParseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Receipt.RequestedDate != nil {
		t.Fatalf("expected null requested_date, got %#v", body.Receipt.RequestedDate)
	}
}

func TestUpdateItemsReturnsUpdatedParse(t *testing.T) {
	repo := newHandlerReceiptRepo()
	repo.jobs[handlerJobID] = &receiptsdomain.Job{
		ID:                  handlerJobID,
		FamilyID:            handlerFamilyID,
		UserID:              handlerUserID,
		Status:              receiptsdomain.StatusReady,
		CategoryMode:        receiptsdomain.CategoryModeSelected,
		SelectedCategoryIDs: []byte(`["` + handlerCategoryID + `"]`),
		Currency:            testStringPtr("BYN"),
	}
	amountA := 10.0
	amountB := 2.0
	repo.items[handlerJobID] = []receiptsdomain.Item{
		{
			ID:              "item-1",
			JobID:           handlerJobID,
			RawName:         "Milk",
			LineTotal:       amountA,
			FinalLineTotal:  &amountA,
			LLMCategoryID:   testStringPtr(handlerCategoryID),
			FinalCategoryID: testStringPtr(handlerCategoryID),
		},
		{
			ID:             "item-2",
			JobID:          handlerJobID,
			RawName:        "Unknown",
			LineTotal:      amountB,
			FinalLineTotal: &amountB,
		},
	}
	repo.drafts[handlerJobID] = []receiptsdomain.DraftExpense{
		{
			ID:         handlerDraftID,
			JobID:      handlerJobID,
			Title:      "Products",
			Amount:     amountA,
			Currency:   "BYN",
			CategoryID: handlerCategoryID,
			Warnings:   []byte("[]"),
		},
	}
	h := newTestHandlers(repo)
	req := authenticatedRequest(http.MethodPatch, "/api/receipt-parses/"+handlerJobID+"/items", strings.NewReader(`{"items":[{"id":"item-2","amount":5.5,"category_id":"`+handlerCategoryID+`"}]}`))
	req = withURLParam(req, "id", handlerJobID)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.UpdateItems(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body receiptParseResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.UnresolvedItems) != 0 {
		t.Fatalf("expected no unresolved items, got %#v", body.UnresolvedItems)
	}
	if len(body.DraftExpenses) != 1 || body.DraftExpenses[0].Amount != 15.5 {
		t.Fatalf("unexpected rebuilt draft expenses: %#v", body.DraftExpenses)
	}
}

func newTestHandlers(repo *handlerReceiptRepo) *Handlers {
	families := familydomain.NewService(&handlerFamilyRepo{
		family: &familydomain.Family{
			ID:              handlerFamilyID,
			Name:            "Family",
			Code:            "ABC123",
			OwnerID:         handlerUserID,
			DefaultCurrency: "BYN",
		},
	})
	receipts := receiptsdomain.NewServiceWithOptions(repo, receiptsdomain.NewMockParser(), handlerCategoryProvider{}, handlerExpenseBatchCreator{}, receiptsdomain.ServiceOptions{
		FileStore:     newHandlerMemoryFileStore(),
		WorkerEnabled: false,
	})
	return New(families, receipts, logger.New(io.Discard, slog.LevelError, "text"))
}

func authenticatedRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	user := middleware.User{ID: handlerUserID, Email: "test@example.com"}
	return req.WithContext(middleware.WithUser(req.Context(), user))
}

func multipartReceiptBody(t *testing.T) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("category_ids", handlerCategoryID); err != nil {
		t.Fatalf("write category field: %v", err)
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="receipt"; filename="receipt.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create receipt file: %v", err)
	}
	if _, err := part.Write([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}); err != nil {
		t.Fatalf("write receipt file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return &body, writer.FormDataContentType()
}

func testStringPtr(value string) *string {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func withURLParam(req *http.Request, key, value string) *http.Request {
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add(key, value)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeContext)
	return req.WithContext(ctx)
}

type handlerFamilyRepo struct {
	family *familydomain.Family
}

func (r *handlerFamilyRepo) Transaction(ctx context.Context, fn func(familydomain.Repository) error) error {
	return fn(r)
}

func (r *handlerFamilyRepo) GetFamilyByUser(context.Context, string) (*familydomain.Family, error) {
	family := *r.family
	return &family, nil
}

func (r *handlerFamilyRepo) GetFamilyByCode(context.Context, string) (*familydomain.Family, error) {
	return nil, familydomain.ErrFamilyNotFound
}

func (r *handlerFamilyRepo) GetMemberByUser(context.Context, string) (*familydomain.FamilyMember, error) {
	return nil, familydomain.ErrFamilyNotFound
}

func (r *handlerFamilyRepo) GetMember(context.Context, string, string) (*familydomain.FamilyMember, error) {
	return nil, familydomain.ErrFamilyNotFound
}

func (r *handlerFamilyRepo) ListMembers(context.Context, string) ([]familydomain.FamilyMember, error) {
	return nil, nil
}

func (r *handlerFamilyRepo) ListMembersWithProfiles(context.Context, string) ([]familydomain.FamilyMemberProfile, error) {
	return nil, nil
}

func (r *handlerFamilyRepo) CreateFamily(context.Context, *familydomain.Family) error {
	return nil
}

func (r *handlerFamilyRepo) AddMember(context.Context, *familydomain.FamilyMember) error {
	return nil
}

func (r *handlerFamilyRepo) UpdateFamilyName(context.Context, string, string) error {
	return nil
}

func (r *handlerFamilyRepo) UpdateFamilyDefaultCurrency(context.Context, string, string) error {
	return nil
}

func (r *handlerFamilyRepo) UpdateFamilyOwner(context.Context, string, string) error {
	return nil
}

func (r *handlerFamilyRepo) UpdateMemberRole(context.Context, string, string, string) error {
	return nil
}

func (r *handlerFamilyRepo) DeleteFamily(context.Context, string) error {
	return nil
}

func (r *handlerFamilyRepo) DeleteMember(context.Context, string, string) error {
	return nil
}

func (r *handlerFamilyRepo) DeleteMembersByFamily(context.Context, string) error {
	return nil
}

func (r *handlerFamilyRepo) CountMembers(context.Context, string) (int64, error) {
	return 1, nil
}

func (r *handlerFamilyRepo) IsUserInFamily(context.Context, string) (bool, error) {
	return true, nil
}

func (r *handlerFamilyRepo) IsCodeTaken(context.Context, string) (bool, error) {
	return false, nil
}

type handlerCategoryProvider struct{}

func (handlerCategoryProvider) ListCategories(context.Context, string) ([]expensesdomain.Category, error) {
	return []expensesdomain.Category{{ID: handlerCategoryID, FamilyID: handlerFamilyID, Name: "Products"}}, nil
}

type handlerExpenseBatchCreator struct{}

func (handlerExpenseBatchCreator) CreateExpensesBatch(context.Context, []expensesdomain.CreateExpenseInput) ([]expensesdomain.ExpenseWithCategories, error) {
	return nil, nil
}

func (handlerExpenseBatchCreator) CreateExpensesBatchWithRepository(context.Context, expensesdomain.Repository, []expensesdomain.CreateExpenseInput) ([]expensesdomain.ExpenseWithCategories, error) {
	return nil, nil
}

type handlerMemoryFileStore struct {
	files map[string][]byte
}

func newHandlerMemoryFileStore() *handlerMemoryFileStore {
	return &handlerMemoryFileStore{files: make(map[string][]byte)}
}

func (s *handlerMemoryFileStore) Save(_ context.Context, jobID, fileID string, file receiptsdomain.UploadedFile) (string, error) {
	key := jobID + "/" + fileID
	s.files[key] = append([]byte{}, file.Data...)
	return key, nil
}

func (s *handlerMemoryFileStore) Load(_ context.Context, storageKey string) ([]byte, error) {
	return append([]byte{}, s.files[storageKey]...), nil
}

func (s *handlerMemoryFileStore) Delete(_ context.Context, storageKey string) error {
	delete(s.files, storageKey)
	return nil
}

type handlerReceiptRepo struct {
	jobs   map[string]*receiptsdomain.Job
	files  map[string][]receiptsdomain.File
	items  map[string][]receiptsdomain.Item
	drafts map[string][]receiptsdomain.DraftExpense
}

func newHandlerReceiptRepo() *handlerReceiptRepo {
	return &handlerReceiptRepo{
		jobs:   make(map[string]*receiptsdomain.Job),
		files:  make(map[string][]receiptsdomain.File),
		items:  make(map[string][]receiptsdomain.Item),
		drafts: make(map[string][]receiptsdomain.DraftExpense),
	}
}

func (r *handlerReceiptRepo) Transaction(ctx context.Context, fn func(receiptsdomain.Repository, expensesdomain.Repository) error) error {
	return fn(r, nil)
}

func (r *handlerReceiptRepo) CreateJob(_ context.Context, job *receiptsdomain.Job) error {
	jobCopy := *job
	r.jobs[job.ID] = &jobCopy
	return nil
}

func (r *handlerReceiptRepo) CreateFile(_ context.Context, file *receiptsdomain.File) error {
	r.files[file.JobID] = append(r.files[file.JobID], *file)
	return nil
}

func (r *handlerReceiptRepo) GetJobByID(_ context.Context, familyID, jobID string) (*receiptsdomain.Job, error) {
	job, ok := r.jobs[jobID]
	if !ok || job.FamilyID != familyID {
		return nil, receiptsdomain.ErrReceiptParseNotFound
	}
	jobCopy := *job
	return &jobCopy, nil
}

func (r *handlerReceiptRepo) GetActiveJob(context.Context, string) (*receiptsdomain.Job, error) {
	return nil, nil
}

func (r *handlerReceiptRepo) CountActiveJobs(_ context.Context, familyID string) (int64, error) {
	var count int64
	for _, job := range r.jobs {
		if job.FamilyID == familyID && handlerActiveStatus(job.Status) {
			count++
		}
	}
	return count, nil
}

func handlerActiveStatus(status receiptsdomain.ParseStatus) bool {
	switch status {
	case receiptsdomain.StatusQueued, receiptsdomain.StatusProcessing, receiptsdomain.StatusReady, receiptsdomain.StatusFailed:
		return true
	default:
		return false
	}
}

func (r *handlerReceiptRepo) AcquireQueuedJob(context.Context, string, time.Time) (*receiptsdomain.Job, error) {
	return nil, nil
}

func (r *handlerReceiptRepo) RequeueStaleProcessing(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (r *handlerReceiptRepo) UpdateJob(_ context.Context, job *receiptsdomain.Job) error {
	jobCopy := *job
	r.jobs[job.ID] = &jobCopy
	return nil
}

func (r *handlerReceiptRepo) ListFilesByJobID(_ context.Context, jobID string) ([]receiptsdomain.File, error) {
	return append([]receiptsdomain.File{}, r.files[jobID]...), nil
}

func (r *handlerReceiptRepo) ListItemsByJobID(_ context.Context, jobID string) ([]receiptsdomain.Item, error) {
	return append([]receiptsdomain.Item{}, r.items[jobID]...), nil
}

func (r *handlerReceiptRepo) ReplaceItems(_ context.Context, jobID string, items []receiptsdomain.Item) error {
	r.items[jobID] = append([]receiptsdomain.Item{}, items...)
	return nil
}

func (r *handlerReceiptRepo) ReplaceDraftExpenses(_ context.Context, jobID string, drafts []receiptsdomain.DraftExpense) error {
	r.drafts[jobID] = append([]receiptsdomain.DraftExpense{}, drafts...)
	return nil
}

func (r *handlerReceiptRepo) ListDraftExpenses(_ context.Context, jobID string) ([]receiptsdomain.DraftExpense, error) {
	return append([]receiptsdomain.DraftExpense{}, r.drafts[jobID]...), nil
}

func (r *handlerReceiptRepo) UpdateItem(_ context.Context, item *receiptsdomain.Item) error {
	items := r.items[item.JobID]
	for index := range items {
		if items[index].ID == item.ID {
			items[index] = *item
			r.items[item.JobID] = items
			return nil
		}
	}
	return receiptsdomain.ErrReceiptParseInvalidStatus
}

func (r *handlerReceiptRepo) UpdateDraftExpense(_ context.Context, draft *receiptsdomain.DraftExpense) error {
	drafts := r.drafts[draft.JobID]
	for index := range drafts {
		if drafts[index].ID == draft.ID {
			drafts[index] = *draft
			r.drafts[draft.JobID] = drafts
			return nil
		}
	}
	return receiptsdomain.ErrReceiptParseInvalidStatus
}

func (r *handlerReceiptRepo) CreateCategoryCorrectionEvent(context.Context, *receiptsdomain.CategoryCorrectionEvent) error {
	return nil
}

func (r *handlerReceiptRepo) AcquireUnprocessedCategoryCorrectionEvent(context.Context, string, time.Time) (*receiptsdomain.CategoryCorrectionEvent, error) {
	return nil, nil
}

func (r *handlerReceiptRepo) RequeueStaleCategoryCorrections(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (r *handlerReceiptRepo) MarkCategoryCorrectionEventProcessed(context.Context, string, time.Time) error {
	return nil
}

func (r *handlerReceiptRepo) ReleaseCategoryCorrectionEventWithError(context.Context, string, string, string, *time.Time) error {
	return nil
}

func (r *handlerReceiptRepo) UpsertFamilyHint(_ context.Context, input receiptsdomain.UpsertFamilyHintInput) (*receiptsdomain.FamilyHint, error) {
	return &receiptsdomain.FamilyHint{
		ID:              input.ID,
		FamilyID:        input.FamilyID,
		CanonicalName:   input.CanonicalName,
		FinalCategoryID: input.FinalCategoryID,
		TimesConfirmed:  1,
		LastConfirmedAt: input.ConfirmedAt,
		CreatedAt:       input.ConfirmedAt,
		UpdatedAt:       input.ConfirmedAt,
	}, nil
}

func (r *handlerReceiptRepo) CreateFamilyHintExample(context.Context, *receiptsdomain.FamilyHintExample) error {
	return nil
}

func (r *handlerReceiptRepo) ListFamilyHints(context.Context, string, []string, int) ([]receiptsdomain.FamilyHint, error) {
	return []receiptsdomain.FamilyHint{}, nil
}
