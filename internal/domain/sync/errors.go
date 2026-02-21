package sync

import "errors"

var (
	ErrBatchTooLarge                 = errors.New("sync batch too large")
	ErrIdempotencyKeyPayloadMismatch = errors.New("idempotency key payload mismatch")
	ErrBatchInProgress               = errors.New("sync batch in progress")
)
