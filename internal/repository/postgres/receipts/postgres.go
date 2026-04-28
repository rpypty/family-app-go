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

func (r *PostgresRepository) CreateCategoryCorrectionEvent(ctx context.Context, event *receiptsdomain.CategoryCorrectionEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

func (r *PostgresRepository) AcquireUnprocessedCategoryCorrectionEvent(ctx context.Context, workerID string, now time.Time) (*receiptsdomain.CategoryCorrectionEvent, error) {
	var acquired *receiptsdomain.CategoryCorrectionEvent
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var event receiptsdomain.CategoryCorrectionEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("processed_at IS NULL").
			Where("locked_at IS NULL").
			Where("next_materialize_attempt_at IS NULL OR next_materialize_attempt_at <= ?", now).
			Order("created_at ASC").
			First(&event).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		event.MaterializeAttemptCount++
		event.LastMaterializeAttemptAt = &now
		event.LockedAt = &now
		event.LockedBy = &workerID
		event.MaterializeErrorCode = nil
		event.MaterializeErrorMessage = nil
		if err := tx.Save(&event).Error; err != nil {
			return err
		}
		acquired = &event
		return nil
	})
	return acquired, err
}

func (r *PostgresRepository) RequeueStaleCategoryCorrections(ctx context.Context, staleBefore time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&receiptsdomain.CategoryCorrectionEvent{}).
		Where("processed_at IS NULL AND locked_at IS NOT NULL AND locked_at < ?", staleBefore).
		Updates(map[string]interface{}{
			"locked_at": nil,
			"locked_by": nil,
		})
	return result.RowsAffected, result.Error
}

func (r *PostgresRepository) MarkCategoryCorrectionEventProcessed(ctx context.Context, eventID string, processedAt time.Time) error {
	return r.db.WithContext(ctx).
		Model(&receiptsdomain.CategoryCorrectionEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"processed_at":                processedAt,
			"locked_at":                   nil,
			"locked_by":                   nil,
			"next_materialize_attempt_at": nil,
			"materialize_error_code":      nil,
			"materialize_error_message":   nil,
		}).Error
}

func (r *PostgresRepository) ReleaseCategoryCorrectionEventWithError(ctx context.Context, eventID, code, message string, nextAttemptAt *time.Time) error {
	return r.db.WithContext(ctx).
		Model(&receiptsdomain.CategoryCorrectionEvent{}).
		Where("id = ?", eventID).
		Updates(map[string]interface{}{
			"locked_at":                   nil,
			"locked_by":                   nil,
			"next_materialize_attempt_at": nextAttemptAt,
			"materialize_error_code":      code,
			"materialize_error_message":   message,
		}).Error
}

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
				"times_confirmed":   gorm.Expr("receipt_parse_family_hints.times_confirmed + 1"),
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

func (r *PostgresRepository) CreateFamilyHintExample(ctx context.Context, example *receiptsdomain.FamilyHintExample) error {
	return r.db.WithContext(ctx).Create(example).Error
}

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

func activeStatuses() []receiptsdomain.ParseStatus {
	return []receiptsdomain.ParseStatus{
		receiptsdomain.StatusQueued,
		receiptsdomain.StatusProcessing,
		receiptsdomain.StatusReady,
		receiptsdomain.StatusFailed,
	}
}
