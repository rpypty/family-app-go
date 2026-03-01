package sync

import (
	"context"
	"errors"

	syncdomain "family-app-go/internal/domain/sync"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) BeginBatch(ctx context.Context, batch *syncdomain.BatchRecord) (bool, *syncdomain.BatchRecord, error) {
	err := r.db.WithContext(ctx).Create(batch).Error
	if err == nil {
		return true, nil, nil
	}
	if !isUniqueViolation(err) {
		return false, nil, err
	}
	if batch.IdempotencyKey == nil {
		return false, nil, nil
	}

	var existing syncdomain.BatchRecord
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND user_id = ? AND idempotency_key = ?", batch.FamilyID, batch.UserID, *batch.IdempotencyKey).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return false, &existing, nil
}

func (r *PostgresRepository) CompleteBatch(ctx context.Context, batchID string, status syncdomain.BatchState, responseJSON []byte) error {
	return r.db.WithContext(ctx).
		Model(&syncdomain.BatchRecord{}).
		Where("id = ?", batchID).
		Updates(map[string]interface{}{
			"status":        status,
			"response_json": responseJSON,
		}).Error
}

func (r *PostgresRepository) ReserveOperation(ctx context.Context, operation *syncdomain.OperationRecord) (bool, *syncdomain.OperationRecord, error) {
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "family_id"},
				{Name: "user_id"},
				{Name: "operation_id"},
			},
			DoNothing: true,
		}).
		Create(operation)
	if result.Error != nil {
		return false, nil, result.Error
	}
	if result.RowsAffected == 1 {
		return true, nil, nil
	}

	var existing syncdomain.OperationRecord
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND user_id = ? AND operation_id = ?", operation.FamilyID, operation.UserID, operation.OperationID).
		First(&existing).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, nil
		}
		return false, nil, err
	}

	return false, &existing, nil
}

func (r *PostgresRepository) UpdateOperation(ctx context.Context, operation *syncdomain.OperationRecord) error {
	return r.db.WithContext(ctx).
		Model(&syncdomain.OperationRecord{}).
		Where("id = ?", operation.ID).
		Updates(map[string]interface{}{
			"status":        operation.Status,
			"local_id":      operation.LocalID,
			"entity":        operation.Entity,
			"server_id":     operation.ServerID,
			"error_code":    operation.ErrorCode,
			"error_message": operation.ErrorMessage,
			"retryable":     operation.Retryable,
		}).Error
}

func (r *PostgresRepository) FindServerIDByLocalID(ctx context.Context, familyID, userID string, entity syncdomain.Entity, localID string) (string, bool, error) {
	type row struct {
		ServerID string `gorm:"column:server_id"`
	}

	var result row
	err := r.db.WithContext(ctx).
		Table("sync_operations").
		Select("server_id").
		Where("family_id = ? AND user_id = ? AND entity = ? AND local_id = ?", familyID, userID, entity, localID).
		Where("status = ?", syncdomain.OperationStateApplied).
		Where("server_id IS NOT NULL").
		Order("created_at DESC").
		Limit(1).
		Scan(&result).Error
	if err != nil {
		return "", false, err
	}
	if result.ServerID == "" {
		return "", false, nil
	}

	return result.ServerID, true, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
