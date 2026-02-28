package expenses

import "context"

type Repository interface {
	Transaction(ctx context.Context, fn func(Repository) error) error
	ListExpenses(ctx context.Context, familyID string, filter ListFilter) ([]Expense, int64, error)
	GetExpenseByID(ctx context.Context, familyID, expenseID string) (*Expense, error)
	CreateExpense(ctx context.Context, expense *Expense) error
	UpdateExpense(ctx context.Context, expense *Expense) error
	DeleteExpense(ctx context.Context, familyID, expenseID string) (bool, error)
	ReplaceExpenseCategories(ctx context.Context, expenseID string, categoryIDs []string) error
	GetCategoryIDsByExpenseIDs(ctx context.Context, expenseIDs []string) (map[string][]string, error)
	CountCategoriesByIDs(ctx context.Context, familyID string, categoryIDs []string) (int64, error)
	ListCategories(ctx context.Context, familyID string) ([]Category, error)
	CreateCategory(ctx context.Context, category *Category) error
	GetCategoryByID(ctx context.Context, familyID, categoryID string) (*Category, error)
	UpdateCategory(ctx context.Context, category *Category) error
	CountCategoriesByName(ctx context.Context, familyID, name, excludeID string) (int64, error)
	DeleteCategory(ctx context.Context, familyID, categoryID string) (bool, error)
	CountExpenseCategoriesByCategoryID(ctx context.Context, categoryID string) (int64, error)
}
