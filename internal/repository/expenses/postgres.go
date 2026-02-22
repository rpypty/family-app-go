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
	if len(filter.TagIDs) > 0 {
		query = query.Joins("join expense_tags on expense_tags.expense_id = expenses.id").Where("expense_tags.tag_id IN ?", filter.TagIDs)
	}

	countQuery := query.Session(&gorm.Session{})
	if len(filter.TagIDs) > 0 {
		countQuery = countQuery.Distinct("expenses.id")
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if len(filter.TagIDs) > 0 {
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

func (r *PostgresRepository) ReplaceExpenseTags(ctx context.Context, expenseID string, tagIDs []string) error {
	if err := r.db.WithContext(ctx).Where("expense_id = ?", expenseID).Delete(&expensesdomain.ExpenseTag{}).Error; err != nil {
		return err
	}

	if len(tagIDs) == 0 {
		return nil
	}

	links := make([]expensesdomain.ExpenseTag, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		links = append(links, expensesdomain.ExpenseTag{ExpenseID: expenseID, TagID: tagID})
	}
	return r.db.WithContext(ctx).Create(&links).Error
}

func (r *PostgresRepository) GetTagIDsByExpenseIDs(ctx context.Context, expenseIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(expenseIDs))
	if len(expenseIDs) == 0 {
		return result, nil
	}

	var rows []struct {
		ExpenseID string `gorm:"column:expense_id"`
		TagID     string `gorm:"column:tag_id"`
	}

	if err := r.db.WithContext(ctx).
		Table("expense_tags").
		Where("expense_id IN ?", expenseIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.ExpenseID] = append(result[row.ExpenseID], row.TagID)
	}

	return result, nil
}

func (r *PostgresRepository) CountTagsByIDs(ctx context.Context, familyID string, tagIDs []string) (int64, error) {
	if len(tagIDs) == 0 {
		return 0, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&expensesdomain.Tag{}).
		Where("family_id = ? AND id IN ?", familyID, tagIDs).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresRepository) ListTags(ctx context.Context, familyID string) ([]expensesdomain.Tag, error) {
	var tags []expensesdomain.Tag
	if err := r.db.WithContext(ctx).
		Where("family_id = ?", familyID).
		Order("created_at asc").
		Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

func (r *PostgresRepository) CreateTag(ctx context.Context, tag *expensesdomain.Tag) error {
	return r.db.WithContext(ctx).Create(tag).Error
}

func (r *PostgresRepository) GetTagByID(ctx context.Context, familyID, tagID string) (*expensesdomain.Tag, error) {
	var tag expensesdomain.Tag
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, tagID).
		First(&tag).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, expensesdomain.ErrTagNotFound
		}
		return nil, err
	}
	return &tag, nil
}

func (r *PostgresRepository) UpdateTag(ctx context.Context, tag *expensesdomain.Tag) error {
	return r.db.WithContext(ctx).
		Model(&expensesdomain.Tag{}).
		Where("id = ? AND family_id = ?", tag.ID, tag.FamilyID).
		Updates(map[string]interface{}{
			"name":  tag.Name,
			"color": tag.Color,
			"emoji": tag.Emoji,
		}).Error
}

func (r *PostgresRepository) CountTagsByName(ctx context.Context, familyID, name, excludeID string) (int64, error) {
	query := r.db.WithContext(ctx).
		Model(&expensesdomain.Tag{}).
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

func (r *PostgresRepository) DeleteTag(ctx context.Context, familyID, tagID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&expensesdomain.Tag{}, "family_id = ? AND id = ?", familyID, tagID)
	return result.RowsAffected > 0, result.Error
}

func (r *PostgresRepository) CountExpenseTagsByTagID(ctx context.Context, tagID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&expensesdomain.ExpenseTag{}).
		Where("tag_id = ?", tagID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
