package expenses

import (
	"context"
	"errors"

	expensesdomain "family-app-go/internal/domain/expenses"
	"gorm.io/gorm"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Transaction(ctx context.Context, fn func(expensesdomain.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PostgresRepository{db: tx})
	})
}

func (r *PostgresRepository) ListExpenses(ctx context.Context, familyID string, filter expensesdomain.ListFilter) ([]expensesdomain.Expense, int64, error) {
	query := r.db.WithContext(ctx).Model(&expensesdomain.Expense{}).Where("family_id = ?", familyID)
	if filter.From != nil {
		query = query.Where("date >= ?", *filter.From)
	}
	if filter.To != nil {
		query = query.Where("date <= ?", *filter.To)
	}
	if len(filter.CategoryIDs) > 0 {
		query = query.Joins("join expense_categories on expense_categories.expense_id = expenses.id").Where("expense_categories.category_id IN ?", filter.CategoryIDs)
	}

	countQuery := query.Session(&gorm.Session{})
	if len(filter.CategoryIDs) > 0 {
		countQuery = countQuery.Distinct("expenses.id")
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if len(filter.CategoryIDs) > 0 {
		query = query.Distinct()
	}

	query = query.Order("date desc, created_at desc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var items []expensesdomain.Expense
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *PostgresRepository) GetExpenseByID(ctx context.Context, familyID, expenseID string) (*expensesdomain.Expense, error) {
	var expense expensesdomain.Expense
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, expenseID).
		First(&expense).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, expensesdomain.ErrExpenseNotFound
		}
		return nil, err
	}
	return &expense, nil
}

func (r *PostgresRepository) CreateExpense(ctx context.Context, expense *expensesdomain.Expense) error {
	return r.db.WithContext(ctx).Create(expense).Error
}

func (r *PostgresRepository) UpdateExpense(ctx context.Context, expense *expensesdomain.Expense) error {
	return r.db.WithContext(ctx).
		Model(&expensesdomain.Expense{}).
		Where("id = ? AND family_id = ?", expense.ID, expense.FamilyID).
		Updates(map[string]interface{}{
			"date":       expense.Date,
			"amount":     expense.Amount,
			"currency":   expense.Currency,
			"title":      expense.Title,
			"updated_at": expense.UpdatedAt,
		}).Error
}

func (r *PostgresRepository) DeleteExpense(ctx context.Context, familyID, expenseID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&expensesdomain.Expense{}, "family_id = ? AND id = ?", familyID, expenseID)
	return result.RowsAffected > 0, result.Error
}

func (r *PostgresRepository) ReplaceExpenseCategories(ctx context.Context, expenseID string, categoryIDs []string) error {
	if err := r.db.WithContext(ctx).Where("expense_id = ?", expenseID).Delete(&expensesdomain.ExpenseCategory{}).Error; err != nil {
		return err
	}

	if len(categoryIDs) == 0 {
		return nil
	}

	links := make([]expensesdomain.ExpenseCategory, 0, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		links = append(links, expensesdomain.ExpenseCategory{ExpenseID: expenseID, CategoryID: categoryID})
	}
	return r.db.WithContext(ctx).Create(&links).Error
}

func (r *PostgresRepository) GetCategoryIDsByExpenseIDs(ctx context.Context, expenseIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(expenseIDs))
	if len(expenseIDs) == 0 {
		return result, nil
	}

	var rows []struct {
		ExpenseID  string `gorm:"column:expense_id"`
		CategoryID string `gorm:"column:category_id"`
	}

	if err := r.db.WithContext(ctx).
		Table("expense_categories").
		Where("expense_id IN ?", expenseIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.ExpenseID] = append(result[row.ExpenseID], row.CategoryID)
	}

	return result, nil
}

func (r *PostgresRepository) CountCategoriesByIDs(ctx context.Context, familyID string, categoryIDs []string) (int64, error) {
	if len(categoryIDs) == 0 {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&expensesdomain.Category{}).
		Where("family_id = ? AND id IN ?", familyID, categoryIDs).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) ListCategories(ctx context.Context, familyID string) ([]expensesdomain.Category, error) {
	var categories []expensesdomain.Category
	if err := r.db.WithContext(ctx).
		Where("family_id = ?", familyID).
		Order("created_at asc").
		Find(&categories).Error; err != nil {
		return nil, err
	}
	return categories, nil
}

func (r *PostgresRepository) CreateCategory(ctx context.Context, category *expensesdomain.Category) error {
	return r.db.WithContext(ctx).Create(category).Error
}

func (r *PostgresRepository) GetCategoryByID(ctx context.Context, familyID, categoryID string) (*expensesdomain.Category, error) {
	var category expensesdomain.Category
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, categoryID).
		First(&category).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, expensesdomain.ErrCategoryNotFound
		}
		return nil, err
	}
	return &category, nil
}

func (r *PostgresRepository) UpdateCategory(ctx context.Context, category *expensesdomain.Category) error {
	return r.db.WithContext(ctx).
		Model(&expensesdomain.Category{}).
		Where("id = ? AND family_id = ?", category.ID, category.FamilyID).
		Updates(map[string]interface{}{
			"name":  category.Name,
			"color": category.Color,
			"emoji": category.Emoji,
		}).Error
}

func (r *PostgresRepository) CountCategoriesByName(ctx context.Context, familyID, name, excludeID string) (int64, error) {
	query := r.db.WithContext(ctx).
		Model(&expensesdomain.Category{}).
		Where("family_id = ? AND lower(name) = lower(?)", familyID, name)
	if excludeID != "" {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) DeleteCategory(ctx context.Context, familyID, categoryID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&expensesdomain.Category{}, "family_id = ? AND id = ?", familyID, categoryID)
	return result.RowsAffected > 0, result.Error
}

func (r *PostgresRepository) CountExpenseCategoriesByCategoryID(ctx context.Context, categoryID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&expensesdomain.ExpenseCategory{}).
		Where("category_id = ?", categoryID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
