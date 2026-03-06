package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"

	analyticsdomain "family-app-go/internal/domain/analytics"
	"gorm.io/gorm"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Summary(ctx context.Context, familyID string, filter analyticsdomain.SummaryFilter) (analyticsdomain.SummaryResult, error) {
	where, args, amountExpr := buildExpenseWhere(familyID, filter.From, filter.To, filter.Currency, filter.UseBaseAmount, filter.CategoryIDs)
	query := "SELECT COALESCE(SUM(" + amountExpr + "), 0) AS total_amount, COUNT(*) AS count FROM expenses e WHERE " + where

	var row struct {
		TotalAmount float64 `gorm:"column:total_amount"`
		Count       int64   `gorm:"column:count"`
	}

	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&row).Error; err != nil {
		return analyticsdomain.SummaryResult{}, err
	}

	return analyticsdomain.SummaryResult{TotalAmount: row.TotalAmount, Count: row.Count}, nil
}

func (r *PostgresRepository) Timeseries(ctx context.Context, familyID string, filter analyticsdomain.TimeseriesFilter) ([]analyticsdomain.TimeseriesPoint, error) {
	where, args, amountExpr := buildExpenseWhere(familyID, filter.From, filter.To, filter.Currency, filter.UseBaseAmount, filter.CategoryIDs)

	groupBy := strings.ToLower(strings.TrimSpace(filter.GroupBy))
	if groupBy != "day" && groupBy != "week" {
		return nil, fmt.Errorf("invalid group_by")
	}

	// e.date is a DATE (calendar day). Applying timezone conversion here shifts
	// bucket boundaries and may move expenses to neighbor days.
	periodExpr := fmt.Sprintf("date_trunc('%s', e.date::timestamp)", groupBy)
	selectExpr := fmt.Sprintf("to_char(%s, 'YYYY-MM-DD')", periodExpr)
	query := fmt.Sprintf("SELECT %s AS period, COALESCE(SUM(%s), 0) AS total, COUNT(*) AS count FROM expenses e WHERE %s GROUP BY 1 ORDER BY 1", selectExpr, amountExpr, where)

	var rows []analyticsdomain.TimeseriesPoint
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func (r *PostgresRepository) ByCategory(ctx context.Context, familyID string, filter analyticsdomain.ByCategoryFilter) ([]analyticsdomain.ByCategoryRow, error) {
	where, args, amountExpr := buildExpenseWhere(familyID, filter.From, filter.To, filter.Currency, filter.UseBaseAmount, nil)
	where = "t.family_id = ? AND " + where
	args = append([]interface{}{familyID}, args...)
	if len(filter.CategoryIDs) > 0 {
		where += " AND t.id IN (?)"
		args = append(args, filter.CategoryIDs)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf("SELECT t.id AS category_id, t.name AS category_name, COALESCE(SUM(%s), 0) AS total, COUNT(e.id) AS count FROM categories t JOIN expense_categories et ON et.category_id = t.id JOIN expenses e ON e.id = et.expense_id WHERE %s GROUP BY t.id, t.name ORDER BY total DESC LIMIT ?", amountExpr, where)
	args = append(args, limit)

	var rows []analyticsdomain.ByCategoryRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func (r *PostgresRepository) TopCategories(ctx context.Context, familyID string, filter analyticsdomain.TopCategoriesFilter) ([]analyticsdomain.ByCategoryRow, int64, error) {
	readLimit := filter.DBReadLimit
	if readLimit <= 0 {
		readLimit = 1000
	}
	responseCount := filter.ResponseCount
	if responseCount <= 0 {
		responseCount = 5
	}

	countQuery := "SELECT COUNT(*) AS records_read FROM (SELECT 1 FROM expenses e WHERE e.family_id = ? AND e.date >= ? AND e.date <= ? ORDER BY e.date DESC, e.created_at DESC LIMIT ?) limited_expenses"
	var countRow struct {
		RecordsRead int64 `gorm:"column:records_read"`
	}
	if err := r.db.WithContext(ctx).Raw(countQuery, familyID, filter.From, filter.To, readLimit).Scan(&countRow).Error; err != nil {
		return nil, 0, err
	}

	query := "WITH limited_expenses AS (" +
		"SELECT e.id, COALESCE(e.amount_in_base, e.amount) AS amount FROM expenses e WHERE e.family_id = ? AND e.date >= ? AND e.date <= ? ORDER BY e.date DESC, e.created_at DESC LIMIT ?" +
		") SELECT c.id AS category_id, c.name AS category_name, COALESCE(SUM(le.amount), 0) AS total, COUNT(le.id) AS count " +
		"FROM limited_expenses le " +
		"JOIN expense_categories ec ON ec.expense_id = le.id " +
		"JOIN categories c ON c.id = ec.category_id AND c.family_id = ? " +
		"GROUP BY c.id, c.name " +
		"ORDER BY count DESC, total DESC " +
		"LIMIT ?"

	var rows []analyticsdomain.ByCategoryRow
	if err := r.db.WithContext(ctx).Raw(query, familyID, filter.From, filter.To, readLimit, familyID, responseCount).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	return rows, countRow.RecordsRead, nil
}

func (r *PostgresRepository) Monthly(ctx context.Context, familyID string, filter analyticsdomain.MonthlyFilter) ([]analyticsdomain.MonthlyRow, error) {
	where, args, amountExpr := buildExpenseWhereRange(familyID, filter.From, filter.To, filter.Currency, filter.UseBaseAmount, filter.CategoryIDs)
	periodExpr := "date_trunc('month', e.date::timestamp)"
	selectExpr := "to_char(" + periodExpr + ", 'YYYY-MM')"
	query := fmt.Sprintf("SELECT %s AS month, COALESCE(SUM(%s), 0) AS total, COUNT(*) AS count FROM expenses e WHERE %s GROUP BY %s ORDER BY %s", selectExpr, amountExpr, where, periodExpr, periodExpr)

	var rows []analyticsdomain.MonthlyRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func buildExpenseWhere(familyID string, from, to time.Time, currency string, useBaseAmount bool, categoryIDs []string) (string, []interface{}, string) {
	conditions := []string{"e.family_id = ?", "e.date >= ?", "e.date <= ?"}
	args := []interface{}{familyID, from, to}
	amountExpr := "e.amount"

	if currency != "" {
		if useBaseAmount {
			conditions = append(conditions, "((e.base_currency = ? AND e.amount_in_base IS NOT NULL) OR (e.currency = ? AND e.amount_in_base IS NULL))")
			args = append(args, currency, currency)
			amountExpr = "COALESCE(e.amount_in_base, e.amount)"
		} else {
			conditions = append(conditions, "e.currency = ?")
			args = append(args, currency)
		}
	}
	if len(categoryIDs) > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM expense_categories et WHERE et.expense_id = e.id AND et.category_id IN (?))")
		args = append(args, categoryIDs)
	}

	return strings.Join(conditions, " AND "), args, amountExpr
}

func buildExpenseWhereRange(familyID string, from, to time.Time, currency string, useBaseAmount bool, categoryIDs []string) (string, []interface{}, string) {
	conditions := []string{"e.family_id = ?", "e.date >= ?", "e.date < ?"}
	args := []interface{}{familyID, from, to}
	amountExpr := "e.amount"

	if currency != "" {
		if useBaseAmount {
			conditions = append(conditions, "((e.base_currency = ? AND e.amount_in_base IS NOT NULL) OR (e.currency = ? AND e.amount_in_base IS NULL))")
			args = append(args, currency, currency)
			amountExpr = "COALESCE(e.amount_in_base, e.amount)"
		} else {
			conditions = append(conditions, "e.currency = ?")
			args = append(args, currency)
		}
	}
	if len(categoryIDs) > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM expense_categories et WHERE et.expense_id = e.id AND et.category_id IN (?))")
		args = append(args, categoryIDs)
	}

	return strings.Join(conditions, " AND "), args, amountExpr
}
