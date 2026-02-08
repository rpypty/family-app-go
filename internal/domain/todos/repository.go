package todos

import "context"

type Repository interface {
	Transaction(ctx context.Context, fn func(Repository) error) error
	LockFamilyOrders(ctx context.Context, familyID string) error
	ListTodoLists(ctx context.Context, familyID string, filter ListFilter) ([]TodoList, int64, error)
	GetTodoListByID(ctx context.Context, familyID, listID string) (*TodoList, error)
	CreateTodoList(ctx context.Context, list *TodoList) error
	UpdateTodoList(ctx context.Context, list *TodoList) error
	SoftDeleteTodoList(ctx context.Context, familyID, listID string) (bool, error)
	GetMaxOrder(ctx context.Context, familyID string) (int, error)
	ShiftOrderRange(ctx context.Context, familyID string, from, to, delta int) error
	SetCompletedItemsArchived(ctx context.Context, listID string, archived bool) error
	SoftDeleteItemsByList(ctx context.Context, listID string) error
	CountItemsByListIDs(ctx context.Context, listIDs []string) (map[string]ListItemCounts, error)
	ListItemsByListIDs(ctx context.Context, listIDs []string, archived ArchivedFilter) ([]TodoItem, error)
	ListTodoItems(ctx context.Context, listID string, archived ArchivedFilter) ([]TodoItem, int64, error)
	CreateTodoItem(ctx context.Context, item *TodoItem) error
	GetTodoItemWithListArchive(ctx context.Context, familyID, itemID string) (*TodoItem, bool, error)
	UpdateTodoItem(ctx context.Context, item *TodoItem) error
	SoftDeleteTodoItem(ctx context.Context, itemID string) (bool, error)
}
