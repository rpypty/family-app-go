package todos

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	todosdomain "family-app-go/internal/domain/todos"
	"gorm.io/gorm"
)

type PostgresRepository struct {
	db *gorm.DB
}

func NewPostgres(db *gorm.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Transaction(ctx context.Context, fn func(todosdomain.Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&PostgresRepository{db: tx})
	})
}

func (r *PostgresRepository) LockFamilyOrders(ctx context.Context, familyID string) error {
	return r.db.WithContext(ctx).
		Exec("SELECT pg_advisory_xact_lock(hashtext(?))", familyID).
		Error
}

func (r *PostgresRepository) ListTodoLists(ctx context.Context, familyID string, filter todosdomain.ListFilter) ([]todosdomain.TodoList, int64, error) {
	query := r.db.WithContext(ctx).Model(&todosdomain.TodoList{}).Where("family_id = ?", familyID)
	search := strings.TrimSpace(filter.Query)
	if search != "" {
		query = query.Where("title ILIKE ?", "%"+search+"%")
	}

	countQuery := query.Session(&gorm.Session{})
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("order_index asc, created_at asc")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var lists []todosdomain.TodoList
	if err := query.Find(&lists).Error; err != nil {
		return nil, 0, err
	}

	return lists, total, nil
}

func (r *PostgresRepository) GetTodoListByID(ctx context.Context, familyID, listID string) (*todosdomain.TodoList, error) {
	var list todosdomain.TodoList
	if err := r.db.WithContext(ctx).
		Where("family_id = ? AND id = ?", familyID, listID).
		First(&list).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, todosdomain.ErrTodoListNotFound
		}
		return nil, err
	}
	return &list, nil
}

func (r *PostgresRepository) CreateTodoList(ctx context.Context, list *todosdomain.TodoList) error {
	return r.db.WithContext(ctx).Create(list).Error
}

func (r *PostgresRepository) UpdateTodoList(ctx context.Context, list *todosdomain.TodoList) error {
	return r.db.WithContext(ctx).
		Model(&todosdomain.TodoList{}).
		Where("id = ? AND family_id = ?", list.ID, list.FamilyID).
		Updates(map[string]interface{}{
			"title":             list.Title,
			"archive_completed": list.ArchiveCompleted,
			"is_collapsed":      list.IsCollapsed,
			"order_index":       list.Order,
		}).Error
}

func (r *PostgresRepository) SoftDeleteTodoList(ctx context.Context, familyID, listID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&todosdomain.TodoList{}, "family_id = ? AND id = ?", familyID, listID)
	return result.RowsAffected > 0, result.Error
}

func (r *PostgresRepository) GetMaxOrder(ctx context.Context, familyID string) (int, error) {
	var max sql.NullInt64
	if err := r.db.WithContext(ctx).
		Model(&todosdomain.TodoList{}).
		Select("MAX(order_index)").
		Where("family_id = ?", familyID).
		Scan(&max).Error; err != nil {
		return 0, err
	}
	if !max.Valid {
		return -1, nil
	}
	return int(max.Int64), nil
}

func (r *PostgresRepository) ShiftOrderRange(ctx context.Context, familyID string, from, to, delta int) error {
	if from > to || delta == 0 {
		return nil
	}

	maxOrder, err := r.GetMaxOrder(ctx, familyID)
	if err != nil {
		return err
	}

	tempOffset := maxOrder + 1 + (to - from + 1)
	if tempOffset < 1 {
		tempOffset = 1
	}

	if err := r.db.WithContext(ctx).
		Model(&todosdomain.TodoList{}).
		Where("family_id = ? AND order_index BETWEEN ? AND ? AND deleted_at IS NULL", familyID, from, to).
		Update("order_index", gorm.Expr("order_index + ?", tempOffset)).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).
		Model(&todosdomain.TodoList{}).
		Where("family_id = ? AND order_index BETWEEN ? AND ? AND deleted_at IS NULL", familyID, from+tempOffset, to+tempOffset).
		Update("order_index", gorm.Expr("order_index - ? + ?", tempOffset, delta)).Error
}

func (r *PostgresRepository) SetCompletedItemsArchived(ctx context.Context, listID string, archived bool) error {
	return r.db.WithContext(ctx).
		Model(&todosdomain.TodoItem{}).
		Where("list_id = ? AND is_completed = ?", listID, true).
		Updates(map[string]interface{}{
			"is_archived": archived,
		}).Error
}

func (r *PostgresRepository) SoftDeleteItemsByList(ctx context.Context, listID string) error {
	return r.db.WithContext(ctx).Delete(&todosdomain.TodoItem{}, "list_id = ?", listID).Error
}

func (r *PostgresRepository) CountItemsByListIDs(ctx context.Context, listIDs []string) (map[string]todosdomain.ListItemCounts, error) {
	result := make(map[string]todosdomain.ListItemCounts, len(listIDs))
	if len(listIDs) == 0 {
		return result, nil
	}

	type row struct {
		ListID         string `gorm:"column:list_id"`
		ItemsTotal     int64  `gorm:"column:items_total"`
		ItemsCompleted int64  `gorm:"column:items_completed"`
		ItemsArchived  int64  `gorm:"column:items_archived"`
	}

	var rows []row
	if err := r.db.WithContext(ctx).
		Model(&todosdomain.TodoItem{}).
		Select(`
			list_id,
			COUNT(*) as items_total,
			SUM(CASE WHEN is_completed THEN 1 ELSE 0 END) as items_completed,
			SUM(CASE WHEN is_archived THEN 1 ELSE 0 END) as items_archived`).
		Where("list_id IN ?", listIDs).
		Group("list_id").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	for _, item := range rows {
		result[item.ListID] = todosdomain.ListItemCounts{
			ItemsTotal:     item.ItemsTotal,
			ItemsCompleted: item.ItemsCompleted,
			ItemsArchived:  item.ItemsArchived,
		}
	}

	return result, nil
}

func (r *PostgresRepository) ListItemsByListIDs(ctx context.Context, listIDs []string, archived todosdomain.ArchivedFilter) ([]todosdomain.TodoItem, error) {
	if len(listIDs) == 0 {
		return []todosdomain.TodoItem{}, nil
	}

	query := r.db.WithContext(ctx).Model(&todosdomain.TodoItem{}).Where("list_id IN ?", listIDs)
	switch archived {
	case todosdomain.ArchivedOnly:
		query = query.Where("is_archived = ?", true)
	case todosdomain.ArchivedExclude:
		query = query.Where("is_archived = ?", false)
	}

	query = query.Order("list_id asc, created_at asc")

	var items []todosdomain.TodoItem
	if err := query.Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PostgresRepository) ListTodoItems(ctx context.Context, listID string, archived todosdomain.ArchivedFilter) ([]todosdomain.TodoItem, int64, error) {
	query := r.db.WithContext(ctx).Model(&todosdomain.TodoItem{}).Where("list_id = ?", listID)
	switch archived {
	case todosdomain.ArchivedOnly:
		query = query.Where("is_archived = ?", true)
	case todosdomain.ArchivedExclude:
		query = query.Where("is_archived = ?", false)
	}

	countQuery := query.Session(&gorm.Session{})
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("created_at asc")
	var items []todosdomain.TodoItem
	if err := query.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (r *PostgresRepository) CreateTodoItem(ctx context.Context, item *todosdomain.TodoItem) error {
	return r.db.WithContext(ctx).Create(item).Error
}

func (r *PostgresRepository) GetTodoItemWithListArchive(ctx context.Context, familyID, itemID string) (*todosdomain.TodoItem, bool, error) {
	type row struct {
		todosdomain.TodoItem
		ListArchiveCompleted bool `gorm:"column:list_archive_completed"`
	}

	var result row
	err := r.db.WithContext(ctx).
		Model(&todosdomain.TodoItem{}).
		Select("todo_items.*, todo_lists.archive_completed as list_archive_completed").
		Joins("join todo_lists on todo_lists.id = todo_items.list_id").
		Where("todo_items.id = ?", itemID).
		Where("todo_lists.family_id = ?", familyID).
		Where("todo_lists.deleted_at IS NULL").
		First(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, todosdomain.ErrTodoItemNotFound
		}
		return nil, false, err
	}

	return &result.TodoItem, result.ListArchiveCompleted, nil
}

func (r *PostgresRepository) UpdateTodoItem(ctx context.Context, item *todosdomain.TodoItem) error {
	return r.db.WithContext(ctx).
		Model(&todosdomain.TodoItem{}).
		Where("id = ? AND list_id = ?", item.ID, item.ListID).
		Updates(map[string]interface{}{
			"title":                   item.Title,
			"is_completed":            item.IsCompleted,
			"is_archived":             item.IsArchived,
			"completed_at":            item.CompletedAt,
			"completed_by_id":         item.CompletedByID,
			"completed_by_name":       item.CompletedByName,
			"completed_by_email":      item.CompletedByEmail,
			"completed_by_avatar_url": item.CompletedByAvatarURL,
		}).Error
}

func (r *PostgresRepository) SoftDeleteTodoItem(ctx context.Context, itemID string) (bool, error) {
	result := r.db.WithContext(ctx).Delete(&todosdomain.TodoItem{}, "id = ?", itemID)
	return result.RowsAffected > 0, result.Error
}
