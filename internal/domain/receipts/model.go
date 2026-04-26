package receipts

import "time"

type ParseStatus string

const (
	StatusQueued     ParseStatus = "queued"
	StatusProcessing ParseStatus = "processing"
	StatusReady      ParseStatus = "ready"
	StatusFailed     ParseStatus = "failed"
	StatusApproved   ParseStatus = "approved"
	StatusCancelled  ParseStatus = "cancelled"
)

type CategoryMode string

const (
	CategoryModeSelected CategoryMode = "selected"
	CategoryModeAll      CategoryMode = "all"
)

type Job struct {
	ID                  string       `gorm:"type:uuid;primaryKey"`
	FamilyID            string       `gorm:"type:uuid;index;not null"`
	UserID              string       `gorm:"type:uuid;index;not null"`
	Status              ParseStatus  `gorm:"not null"`
	CategoryMode        CategoryMode `gorm:"not null"`
	SelectedCategoryIDs []byte       `gorm:"type:jsonb;not null"`
	RequestedDate       *time.Time   `gorm:"type:date"`
	RequestedCurrency   *string      `gorm:"type:text"`
	MerchantName        *string      `gorm:"type:text"`
	PurchasedAt         *time.Time   `gorm:"type:date"`
	Currency            *string      `gorm:"type:text"`
	DetectedTotal       *float64     `gorm:"type:numeric(12,2)"`
	ItemsTotal          *float64     `gorm:"type:numeric(12,2)"`
	Provider            *string      `gorm:"type:text"`
	Model               *string      `gorm:"type:text"`
	RawLLMResponse      []byte       `gorm:"type:jsonb"`
	ErrorCode           *string      `gorm:"type:text"`
	ErrorMessage        *string      `gorm:"type:text"`
	AttemptCount        int          `gorm:"not null"`
	LastAttemptAt       *time.Time
	NextAttemptAt       *time.Time
	LockedAt            *time.Time
	LockedBy            *string   `gorm:"type:text"`
	CreatedAt           time.Time `gorm:"autoCreateTime"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime"`
	CompletedAt         *time.Time
	ApprovedAt          *time.Time
	CancelledAt         *time.Time
}

func (Job) TableName() string {
	return "receipt_parse_jobs"
}

type File struct {
	ID          string    `gorm:"type:uuid;primaryKey"`
	JobID       string    `gorm:"type:uuid;index;not null"`
	Ordinal     int       `gorm:"not null"`
	FileName    string    `gorm:"not null"`
	ContentType string    `gorm:"not null"`
	SizeBytes   int64     `gorm:"not null"`
	StorageKey  *string   `gorm:"type:text"`
	SHA256      *string   `gorm:"type:text"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

func (File) TableName() string {
	return "receipt_parse_files"
}

type Item struct {
	ID                    string    `gorm:"type:uuid;primaryKey"`
	JobID                 string    `gorm:"type:uuid;index;not null"`
	LineIndex             int       `gorm:"not null"`
	RawName               string    `gorm:"not null"`
	NormalizedName        *string   `gorm:"type:text"`
	Quantity              *float64  `gorm:"type:numeric(12,3)"`
	UnitPrice             *float64  `gorm:"type:numeric(12,2)"`
	LineTotal             float64   `gorm:"type:numeric(12,2);not null"`
	LLMCategoryID         *string   `gorm:"type:uuid"`
	LLMCategoryConfidence *float64  `gorm:"type:numeric(4,3)"`
	FinalCategoryID       *string   `gorm:"type:uuid"`
	FinalLineTotal        *float64  `gorm:"type:numeric(12,2)"`
	IsDeleted             bool      `gorm:"not null"`
	EditedByUser          bool      `gorm:"not null"`
	CreatedAt             time.Time `gorm:"autoCreateTime"`
}

func (Item) TableName() string {
	return "receipt_parse_items"
}

type DraftExpense struct {
	ID              string    `gorm:"type:uuid;primaryKey"`
	JobID           string    `gorm:"type:uuid;index;not null"`
	Title           string    `gorm:"not null"`
	Amount          float64   `gorm:"type:numeric(12,2);not null"`
	Currency        string    `gorm:"not null"`
	CategoryID      string    `gorm:"type:uuid;not null"`
	Confidence      *float64  `gorm:"type:numeric(4,3)"`
	Warnings        []byte    `gorm:"type:jsonb;not null"`
	FinalTitle      *string   `gorm:"type:text"`
	FinalAmount     *float64  `gorm:"type:numeric(12,2)"`
	FinalCategoryID *string   `gorm:"type:uuid"`
	IsDeleted       bool      `gorm:"not null"`
	EditedByUser    bool      `gorm:"not null"`
	CreatedAt       time.Time `gorm:"autoCreateTime"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime"`
}

func (DraftExpense) TableName() string {
	return "receipt_parse_draft_expenses"
}

type JobWithDrafts struct {
	Job
	DraftExpenses []DraftExpense
	Items         []Item
}

type Category struct {
	ID   string
	Name string
}

type UploadedFile struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	SHA256      string
	Data        []byte
}

type CreateParseInput struct {
	FamilyID            string
	UserID              string
	CategoryMode        CategoryMode
	SelectedCategoryIDs []string
	RequestedDate       *time.Time
	RequestedCurrency   string
	File                UploadedFile
}

type ApproveExpenseInput struct {
	DraftID     string
	Date        time.Time
	Title       string
	Amount      float64
	Currency    string
	CategoryIDs []string
}

type ApproveInput struct {
	FamilyID     string
	UserID       string
	BaseCurrency string
	JobID        string
	Expenses     []ApproveExpenseInput
}

type ReviewItemInput struct {
	ItemID     string
	Amount     *float64
	CategoryID *string
}

type UpdateItemsInput struct {
	FamilyID string
	JobID    string
	Items    []ReviewItemInput
}

type ParseReceiptInput struct {
	File       UploadedFile
	Categories []Category
	Date       *time.Time
	Currency   string
}

type ParsedReceipt struct {
	MerchantName  *string
	PurchasedAt   *time.Time
	Currency      string
	DetectedTotal *float64
	Items         []ParsedItem
	Warnings      []string
	Provider      string
	Model         string
	RawResponse   []byte
}

type ParsedItem struct {
	RawName            string
	NormalizedName     *string
	Quantity           *float64
	UnitPrice          *float64
	LineTotal          float64
	CategoryID         *string
	CategoryConfidence *float64
}
