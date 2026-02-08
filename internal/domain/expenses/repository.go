package expenses

import "context"

type Repository interface {
	Transaction(ctx context.Context, fn func(Repository) error) error
	ListExpenses(ctx context.Context, familyID string, filter ListFilter) ([]Expense, int64, error)
	GetExpenseByID(ctx context.Context, familyID, expenseID string) (*Expense, error)
	CreateExpense(ctx context.Context, expense *Expense) error
	UpdateExpense(ctx context.Context, expense *Expense) error
	DeleteExpense(ctx context.Context, familyID, expenseID string) (bool, error)
	ReplaceExpenseTags(ctx context.Context, expenseID string, tagIDs []string) error
	GetTagIDsByExpenseIDs(ctx context.Context, expenseIDs []string) (map[string][]string, error)
	CountTagsByIDs(ctx context.Context, familyID string, tagIDs []string) (int64, error)
	ListTags(ctx context.Context, familyID string) ([]Tag, error)
	CreateTag(ctx context.Context, tag *Tag) error
	GetTagByID(ctx context.Context, familyID, tagID string) (*Tag, error)
	UpdateTag(ctx context.Context, tag *Tag) error
	CountTagsByName(ctx context.Context, familyID, name, excludeID string) (int64, error)
	DeleteTag(ctx context.Context, familyID, tagID string) (bool, error)
	CountExpenseTagsByTagID(ctx context.Context, tagID string) (int64, error)
}
