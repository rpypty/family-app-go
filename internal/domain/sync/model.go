package sync

import "time"

const MaxBatchOperations = 100

type OperationType string

const (
	OperationTypeCreateExpense    OperationType = "create_expense"
	OperationTypeCreateTodo       OperationType = "create_todo"
	OperationTypeSetTodoCompleted OperationType = "set_todo_completed"
)

type ResultStatus string

const (
	ResultStatusApplied   ResultStatus = "applied"
	ResultStatusDuplicate ResultStatus = "duplicate"
	ResultStatusFailed    ResultStatus = "failed"
)

type BatchStatus string

const (
	BatchStatusSuccess        BatchStatus = "success"
	BatchStatusPartialSuccess BatchStatus = "partial_success"
	BatchStatusFailed         BatchStatus = "failed"
)

type ErrorCode string

const (
	ErrorCodeInvalidRequest                ErrorCode = "invalid_request"
	ErrorCodeInvalidJSON                   ErrorCode = "invalid_json"
	ErrorCodeUnsupportedOperationType      ErrorCode = "unsupported_operation_type"
	ErrorCodeOperationPayloadMismatch      ErrorCode = "operation_payload_mismatch"
	ErrorCodeDependencyNotResolved         ErrorCode = "dependency_not_resolved"
	ErrorCodeTagNotFound                   ErrorCode = "tag_not_found"
	ErrorCodeTodoListNotFound              ErrorCode = "todo_list_not_found"
	ErrorCodeTodoItemNotFound              ErrorCode = "todo_item_not_found"
	ErrorCodeFamilyNotFound                ErrorCode = "family_not_found"
	ErrorCodeSyncBatchTooLarge             ErrorCode = "sync_batch_too_large"
	ErrorCodeIdempotencyKeyPayloadMismatch ErrorCode = "idempotency_key_payload_mismatch"
	ErrorCodeBatchInProgress               ErrorCode = "batch_in_progress"
	ErrorCodeInternalError                 ErrorCode = "internal_error"
)

type Entity string

const (
	EntityExpense  Entity = "expense"
	EntityTodoItem Entity = "todo_item"
)

type BatchState string

const (
	BatchStateProcessing BatchState = "processing"
	BatchStateCompleted  BatchState = "completed"
)

type OperationState string

const (
	OperationStatePending OperationState = "pending"
	OperationStateApplied OperationState = "applied"
	OperationStateFailed  OperationState = "failed"
)

type UserSnapshot struct {
	ID        string
	Name      string
	Email     string
	AvatarURL string
}

type BatchInput struct {
	FamilyID       string
	User           UserSnapshot
	IdempotencyKey string
	Operations     []OperationInput
}

type OperationInput struct {
	OperationID      string
	Type             OperationType
	LocalID          string
	CreateExpense    *CreateExpensePayload
	CreateTodo       *CreateTodoPayload
	SetTodoCompleted *SetTodoCompletedPayload
}

type CreateExpensePayload struct {
	Date     time.Time
	Amount   float64
	Currency string
	Title    string
	TagIDs   []string
}

type CreateTodoPayload struct {
	ListID string
	Title  string
}

type SetTodoCompletedPayload struct {
	TodoID      string
	TodoLocalID string
	IsCompleted bool
}

type BatchResponse struct {
	SyncID     string            `json:"sync_id"`
	Status     BatchStatus       `json:"status"`
	Summary    BatchSummary      `json:"summary"`
	Results    []OperationResult `json:"results"`
	Mappings   []EntityMapping   `json:"mappings"`
	ServerTime time.Time         `json:"server_time"`
}

type BatchSummary struct {
	Total     int `json:"total"`
	Applied   int `json:"applied"`
	Duplicate int `json:"duplicate"`
	Failed    int `json:"failed"`
}

type OperationResult struct {
	OperationID string          `json:"operation_id"`
	Type        OperationType   `json:"type"`
	Status      ResultStatus    `json:"status"`
	LocalID     *string         `json:"local_id,omitempty"`
	Entity      *Entity         `json:"entity,omitempty"`
	ServerID    *string         `json:"server_id,omitempty"`
	Error       *OperationError `json:"error,omitempty"`
}

type OperationError struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

type EntityMapping struct {
	Entity   Entity `json:"entity"`
	LocalID  string `json:"local_id"`
	ServerID string `json:"server_id"`
}

type BatchRecord struct {
	ID             string     `gorm:"type:uuid;primaryKey"`
	FamilyID       string     `gorm:"type:uuid;not null;index"`
	UserID         string     `gorm:"type:uuid;not null;index"`
	IdempotencyKey *string    `gorm:"column:idempotency_key"`
	RequestHash    string     `gorm:"not null"`
	Status         BatchState `gorm:"not null"`
	ResponseJSON   []byte     `gorm:"type:jsonb;column:response_json"`
	CreatedAt      time.Time  `gorm:"autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime"`
}

func (BatchRecord) TableName() string {
	return "sync_batches"
}

type OperationRecord struct {
	ID            string         `gorm:"type:uuid;primaryKey"`
	FamilyID      string         `gorm:"type:uuid;not null;index"`
	UserID        string         `gorm:"type:uuid;not null;index"`
	OperationID   string         `gorm:"type:uuid;not null"`
	OperationType OperationType  `gorm:"not null;column:operation_type"`
	PayloadHash   string         `gorm:"not null;column:payload_hash"`
	LocalID       *string        `gorm:"column:local_id"`
	Status        OperationState `gorm:"not null"`
	Entity        *Entity        `gorm:"column:entity"`
	ServerID      *string        `gorm:"type:uuid;column:server_id"`
	ErrorCode     *ErrorCode     `gorm:"column:error_code"`
	ErrorMessage  *string        `gorm:"column:error_message"`
	Retryable     *bool          `gorm:"column:retryable"`
	CreatedAt     time.Time      `gorm:"autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime"`
}

func (OperationRecord) TableName() string {
	return "sync_operations"
}
