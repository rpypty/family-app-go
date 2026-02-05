package expenses

import "time"

type Expense struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	FamilyID  string    `gorm:"type:uuid;index;not null"`
	UserID    string    `gorm:"type:uuid;index;not null"`
	Date      time.Time `gorm:"type:date;not null"`
	Amount    float64   `gorm:"type:numeric(12,2);not null"`
	Currency  string    `gorm:"size:3;not null"`
	Title     string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type Tag struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	FamilyID  string    `gorm:"type:uuid;index;not null"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

type ExpenseTag struct {
	ExpenseID string `gorm:"type:uuid;primaryKey"`
	TagID     string `gorm:"type:uuid;primaryKey"`
}

type ExpenseWithTags struct {
	Expense
	TagIDs []string
}

type ListFilter struct {
	From   *time.Time
	To     *time.Time
	TagID  string
	Limit  int
	Offset int
}

type CreateExpenseInput struct {
	FamilyID string
	UserID   string
	Date     time.Time
	Amount   float64
	Currency string
	Title    string
	TagIDs   []string
}

type UpdateExpenseInput struct {
	ID       string
	FamilyID string
	Date     time.Time
	Amount   float64
	Currency string
	Title    string
	TagIDs   []string
}
