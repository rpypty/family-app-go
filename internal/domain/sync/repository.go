package sync

import "context"

type Repository interface {
	BeginBatch(ctx context.Context, batch *BatchRecord) (bool, *BatchRecord, error)
	CompleteBatch(ctx context.Context, batchID string, status BatchState, responseJSON []byte) error
	ReserveOperation(ctx context.Context, operation *OperationRecord) (bool, *OperationRecord, error)
	UpdateOperation(ctx context.Context, operation *OperationRecord) error
	FindServerIDByLocalID(ctx context.Context, familyID, userID string, entity Entity, localID string) (string, bool, error)
}
