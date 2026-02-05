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
	where, args := buildExpenseWhere(familyID, filter.From, filter.To, filter.Currency, filter.TagIDs)
	query := "SELECT COALESCE(SUM(e.amount), 0) AS total_amount, COUNT(*) AS count FROM expenses e WHERE " + where

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
	where, args := buildExpenseWhere(familyID, filter.From, filter.To, filter.Currency, filter.TagIDs)

	groupBy := strings.ToLower(strings.TrimSpace(filter.GroupBy))
	format := "YYYY-MM-DD"
	if groupBy == "month" {
		format = "YYYY-MM"
	}
	if groupBy != "day" && groupBy != "week" && groupBy != "month" {
		return nil, fmt.Errorf("invalid group_by")
	}

	tz := strings.TrimSpace(filter.Timezone)
	if tz == "" {
		tz = "UTC"
	}

	periodExpr := fmt.Sprintf("date_trunc('%s', timezone(?, e.date::timestamp))", groupBy)
	selectExpr := fmt.Sprintf("to_char(%s, '%s')", periodExpr, format)
	query := fmt.Sprintf("SELECT %s AS period, COALESCE(SUM(e.amount), 0) AS total, COUNT(*) AS count FROM expenses e WHERE %s GROUP BY %s ORDER BY %s", selectExpr, where, periodExpr, periodExpr)

	queryArgs := []interface{}{tz}
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, tz)

	var rows []analyticsdomain.TimeseriesPoint
	if err := r.db.WithContext(ctx).Raw(query, queryArgs...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func (r *PostgresRepository) ByTag(ctx context.Context, familyID string, filter analyticsdomain.ByTagFilter) ([]analyticsdomain.ByTagRow, error) {
	conditions := []string{"e.family_id = ?", "t.family_id = ?", "e.date >= ?", "e.date <= ?"}
	args := []interface{}{familyID, familyID, filter.From, filter.To}

	if filter.Currency != "" {
		conditions = append(conditions, "e.currency = ?")
		args = append(args, filter.Currency)
	}
	if len(filter.TagIDs) > 0 {
		conditions = append(conditions, "t.id IN (?)")
		args = append(args, filter.TagIDs)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf("SELECT t.id AS tag_id, t.name AS tag_name, COALESCE(SUM(e.amount), 0) AS total, COUNT(e.id) AS count FROM tags t JOIN expense_tags et ON et.tag_id = t.id JOIN expenses e ON e.id = et.expense_id WHERE %s GROUP BY t.id, t.name ORDER BY total DESC LIMIT ?", strings.Join(conditions, " AND "))
	args = append(args, limit)

	var rows []analyticsdomain.ByTagRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func (r *PostgresRepository) Monthly(ctx context.Context, familyID string, filter analyticsdomain.MonthlyFilter) ([]analyticsdomain.MonthlyRow, error) {
	where, args := buildExpenseWhereRange(familyID, filter.From, filter.To, filter.Currency, filter.TagIDs)
	periodExpr := "date_trunc('month', e.date::timestamp)"
	selectExpr := "to_char(" + periodExpr + ", 'YYYY-MM')"
	query := fmt.Sprintf("SELECT %s AS month, COALESCE(SUM(e.amount), 0) AS total, COUNT(*) AS count FROM expenses e WHERE %s GROUP BY %s ORDER BY %s", selectExpr, where, periodExpr, periodExpr)

	var rows []analyticsdomain.MonthlyRow
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return rows, nil
}

func buildExpenseWhere(familyID string, from, to time.Time, currency string, tagIDs []string) (string, []interface{}) {
	conditions := []string{"e.family_id = ?", "e.date >= ?", "e.date <= ?"}
	args := []interface{}{familyID, from, to}

	if currency != "" {
		conditions = append(conditions, "e.currency = ?")
		args = append(args, currency)
	}
	if len(tagIDs) > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM expense_tags et WHERE et.expense_id = e.id AND et.tag_id IN (?))")
		args = append(args, tagIDs)
	}

	return strings.Join(conditions, " AND "), args
}

func buildExpenseWhereRange(familyID string, from, to time.Time, currency string, tagIDs []string) (string, []interface{}) {
	conditions := []string{"e.family_id = ?", "e.date >= ?", "e.date < ?"}
	args := []interface{}{familyID, from, to}

	if currency != "" {
		conditions = append(conditions, "e.currency = ?")
		args = append(args, currency)
	}
	if len(tagIDs) > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM expense_tags et WHERE et.expense_id = e.id AND et.tag_id IN (?))")
		args = append(args, tagIDs)
	}

	return strings.Join(conditions, " AND "), args
}
