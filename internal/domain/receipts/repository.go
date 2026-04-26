package receipts

import (
	"context"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
)

type Repository interface {
	Transaction(ctx context.Context, fn func(Repository, expensesdomain.Repository) error) error
	CreateJob(ctx context.Context, job *Job) error
	CreateFile(ctx context.Context, file *File) error
	GetJobByID(ctx context.Context, familyID, jobID string) (*Job, error)
	GetActiveJob(ctx context.Context, familyID string) (*Job, error)
	CountActiveJobs(ctx context.Context, familyID string) (int64, error)
	AcquireQueuedJob(ctx context.Context, workerID string, now time.Time) (*Job, error)
	RequeueStaleProcessing(ctx context.Context, staleBefore time.Time) (int64, error)
	UpdateJob(ctx context.Context, job *Job) error
	ListFilesByJobID(ctx context.Context, jobID string) ([]File, error)
	ListItemsByJobID(ctx context.Context, jobID string) ([]Item, error)
	ReplaceItems(ctx context.Context, jobID string, items []Item) error
	ReplaceDraftExpenses(ctx context.Context, jobID string, drafts []DraftExpense) error
	ListDraftExpenses(ctx context.Context, jobID string) ([]DraftExpense, error)
	UpdateItem(ctx context.Context, item *Item) error
	UpdateDraftExpense(ctx context.Context, draft *DraftExpense) error
}
