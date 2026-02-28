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

type Category struct {
	ID        string    `gorm:"type:uuid;primaryKey"`
	FamilyID  string    `gorm:"type:uuid;index;not null"`
	Name      string    `gorm:"not null"`
	Color     *string   `gorm:"type:text"`
	Emoji     *string   `gorm:"type:text"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

type ExpenseCategory struct {
	ExpenseID  string `gorm:"type:uuid;primaryKey"`
	CategoryID string `gorm:"type:uuid;primaryKey"`
}

type ExpenseWithCategories struct {
	Expense
	CategoryIDs []string
}

type ListFilter struct {
	From        *time.Time
	To          *time.Time
	CategoryIDs []string
	Limit       int
	Offset      int
}

type CreateExpenseInput struct {
	FamilyID    string
	UserID      string
	Date        time.Time
	Amount      float64
	Currency    string
	Title       string
	CategoryIDs []string
}

type UpdateExpenseInput struct {
	ID          string
	FamilyID    string
	Date        time.Time
	Amount      float64
	Currency    string
	Title       string
	CategoryIDs []string
}

type CreateCategoryInput struct {
	FamilyID string
	Name     string
	Color    *string
	Emoji    *string
}

type OptionalNullableString struct {
	Set   bool
	Value *string
}

type UpdateCategoryInput struct {
	FamilyID   string
	CategoryID string
	Name       string
	Color      OptionalNullableString
	Emoji      OptionalNullableString
}
