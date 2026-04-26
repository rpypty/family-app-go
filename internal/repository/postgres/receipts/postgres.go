package receipts

import (
	"context"
	"errors"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
	receiptsdomain "family-app-go/internal/domain/receipts"
	expensesrepo "family-app-go/internal/repository/postgres/expenses"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Transaction(ctx context.Context, fn func(receiptsdomain.Repository, expensesdomain.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PostgresRepository{db: tx}, expensesrepo.NewPostgres(tx))
	})
}

func (r *PostgresRepository) CreateJob(ctx context.Context, job *receiptsdomain.Job) error {
	return r.db.WithContext(ctx).Create(job).Error
}

func (r *PostgresRepository) CreateFile(ctx context.Context, file *receiptsdomain.File) error {
	return r.db.WithContext(ctx).Create(file).Error
}

func (r *PostgresRepository) GetJobByID(ctx context.Context, familyID, jobID string) (*receiptsdomain.Job, error) {
	var job receiptsdomain.Job
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, jobID).
		First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, receiptsdomain.ErrReceiptParseNotFound
		}
		return nil, err
	}
	return &job, nil
}

func (r *PostgresRepository) GetActiveJob(ctx context.Context, familyID string) (*receiptsdomain.Job, error) {
	var job receiptsdomain.Job
	err := r.db.WithContext(ctx).
		Where("family_id = ? AND status IN ?", familyID, activeStatuses()).
		Order("created_at DESC").
		First(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

func (r *PostgresRepository) CountActiveJobs(ctx context.Context, familyID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&receiptsdomain.Job{}).
		Where("family_id = ? AND status IN ?", familyID, activeStatuses()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) AcquireQueuedJob(ctx context.Context, workerID string, now time.Time) (*receiptsdomain.Job, error) {
	var acquired *receiptsdomain.Job
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job receiptsdomain.Job
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ?", receiptsdomain.StatusQueued).
			Where("next_attempt_at IS NULL OR next_attempt_at <= ?", now).
			Order("created_at ASC").
			First(&job).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		job.Status = receiptsdomain.StatusProcessing
		job.AttemptCount++
		job.LastAttemptAt = &now
		job.LockedAt = &now
		job.LockedBy = &workerID
		job.UpdatedAt = now
		job.ErrorCode = nil
		job.ErrorMessage = nil
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		acquired = &job
		return nil
	})
	return acquired, err
}

func (r *PostgresRepository) RequeueStaleProcessing(ctx context.Context, staleBefore time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&receiptsdomain.Job{}).
		Where("status = ? AND locked_at IS NOT NULL AND locked_at < ?", receiptsdomain.StatusProcessing, staleBefore).
		Updates(map[string]interface{}{
			"status":     receiptsdomain.StatusQueued,
			"locked_at":  nil,
			"locked_by":  nil,
			"updated_at": time.Now().UTC(),
		})
	return result.RowsAffected, result.Error
}

func (r *PostgresRepository) UpdateJob(ctx context.Context, job *receiptsdomain.Job) error {
	return r.db.WithContext(ctx).Save(job).Error
}

func (r *PostgresRepository) ListFilesByJobID(ctx context.Context, jobID string) ([]receiptsdomain.File, error) {
	var files []receiptsdomain.File
	if err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("ordinal ASC").
		Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}

func (r *PostgresRepository) ListItemsByJobID(ctx context.Context, jobID string) ([]receiptsdomain.Item, error) {
	var items []receiptsdomain.Item
	if err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("line_index ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PostgresRepository) ReplaceItems(ctx context.Context, jobID string, items []receiptsdomain.Item) error {
	if err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Delete(&receiptsdomain.Item{}).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&items).Error
}

func (r *PostgresRepository) ReplaceDraftExpenses(ctx context.Context, jobID string, drafts []receiptsdomain.DraftExpense) error {
	if err := r.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Delete(&receiptsdomain.DraftExpense{}).Error; err != nil {
		return err
	}
	if len(drafts) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&drafts).Error
}

func (r *PostgresRepository) ListDraftExpenses(ctx context.Context, jobID string) ([]receiptsdomain.DraftExpense, error) {
	var drafts []receiptsdomain.DraftExpense
	if err := r.db.WithContext(ctx).
		Where("job_id = ? AND is_deleted = false", jobID).
		Order("created_at ASC").
		Find(&drafts).Error; err != nil {
		return nil, err
	}
	return drafts, nil
}

func (r *PostgresRepository) UpdateItem(ctx context.Context, item *receiptsdomain.Item) error {
	return r.db.WithContext(ctx).Save(item).Error
}

func (r *PostgresRepository) UpdateDraftExpense(ctx context.Context, draft *receiptsdomain.DraftExpense) error {
	return r.db.WithContext(ctx).Save(draft).Error
}

func activeStatuses() []receiptsdomain.ParseStatus {
	return []receiptsdomain.ParseStatus{
		receiptsdomain.StatusQueued,
		receiptsdomain.StatusProcessing,
		receiptsdomain.StatusReady,
		receiptsdomain.StatusFailed,
	}
}
