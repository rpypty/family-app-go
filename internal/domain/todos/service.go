package todos

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListTodoLists(ctx context.Context, familyID string, filter ListFilter, includeItems bool, itemsArchived ArchivedFilter) ([]ListWithItems, int64, error) {
	lists, total, err := s.repo.ListTodoLists(ctx, familyID, filter)
	if err != nil {
		return nil, 0, err
	}
	if len(lists) == 0 {
		return []ListWithItems{}, total, nil
	}

	listIDs := make([]string, 0, len(lists))
	for _, list := range lists {
		listIDs = append(listIDs, list.ID)
	}

	counts, err := s.repo.CountItemsByListIDs(ctx, listIDs)
	if err != nil {
		return nil, 0, err
	}

	itemsByList := map[string][]TodoItem{}
	if includeItems {
		items, err := s.repo.ListItemsByListIDs(ctx, listIDs, itemsArchived)
		if err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			itemsByList[item.ListID] = append(itemsByList[item.ListID], item)
		}
	}

	result := make([]ListWithItems, 0, len(lists))
	for _, list := range lists {
		listCounts := counts[list.ID]
		items := itemsByList[list.ID]
		if includeItems && items == nil {
			items = []TodoItem{}
		}
		result = append(result, ListWithItems{
			List:   list,
			Counts: listCounts,
			Items:  items,
		})
	}

	return result, total, nil
}

func (s *Service) CountItemsByListID(ctx context.Context, listID string) (ListItemCounts, error) {
	counts, err := s.repo.CountItemsByListIDs(ctx, []string{listID})
	if err != nil {
		return ListItemCounts{}, err
	}
	return counts[listID], nil
}

func (s *Service) CreateTodoList(ctx context.Context, input CreateTodoListInput) (*TodoList, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	list := TodoList{
		ID:               id,
		FamilyID:         input.FamilyID,
		Title:            title,
		ArchiveCompleted: input.ArchiveCompleted,
	}

	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if err := tx.LockFamilyOrders(ctx, input.FamilyID); err != nil {
			return err
		}
		maxOrder, err := tx.GetMaxOrder(ctx, input.FamilyID)
		if err != nil {
			return err
		}

		order := maxOrder + 1
		if input.Order != nil {
			if *input.Order < 0 {
				return fmt.Errorf("order must be non-negative")
			}
			if *input.Order <= maxOrder {
				order = *input.Order
				if err := tx.ShiftOrderRange(ctx, input.FamilyID, order, maxOrder, 1); err != nil {
					return err
				}
			} else if *input.Order == maxOrder+1 {
				order = *input.Order
			}
		}

		list.Order = order
		return tx.CreateTodoList(ctx, &list)
	})
	if err != nil {
		return nil, err
	}

	return &list, nil
}

func (s *Service) UpdateTodoList(ctx context.Context, input UpdateTodoListInput) (*TodoList, error) {
	if input.Title == nil && input.ArchiveCompleted == nil && input.IsCollapsed == nil && input.Order == nil {
		return nil, fmt.Errorf("no fields to update")
	}

	list, err := s.repo.GetTodoListByID(ctx, input.FamilyID, input.ID)
	if err != nil {
		return nil, err
	}

	archiveChanged := false
	var desiredOrder *int
	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" {
			return nil, fmt.Errorf("title is required")
		}
		list.Title = trimmed
	}
	if input.ArchiveCompleted != nil {
		archiveChanged = list.ArchiveCompleted != *input.ArchiveCompleted
		list.ArchiveCompleted = *input.ArchiveCompleted
	}
	if input.IsCollapsed != nil {
		list.IsCollapsed = *input.IsCollapsed
	}
	if input.Order != nil {
		if *input.Order < 0 {
			return nil, fmt.Errorf("order must be non-negative")
		}
		desiredOrder = input.Order
	}

	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if desiredOrder != nil {
			if err := tx.LockFamilyOrders(ctx, input.FamilyID); err != nil {
				return err
			}
		}
		// Ensure we work with the latest order inside the transaction.
		current, err := tx.GetTodoListByID(ctx, input.FamilyID, input.ID)
		if err != nil {
			return err
		}
		list.Order = current.Order

		if desiredOrder != nil {
			newOrder := *desiredOrder
			maxOrder, err := tx.GetMaxOrder(ctx, input.FamilyID)
			if err != nil {
				return err
			}

			if newOrder > maxOrder {
				newOrder = maxOrder
			}

			if newOrder != list.Order {
				tempOrder := maxOrder + 1
				list.Order = tempOrder
				if err := tx.UpdateTodoList(ctx, list); err != nil {
					return err
				}

				if newOrder > current.Order {
					if err := tx.ShiftOrderRange(ctx, input.FamilyID, current.Order+1, newOrder, -1); err != nil {
						return err
					}
				} else {
					if err := tx.ShiftOrderRange(ctx, input.FamilyID, newOrder, current.Order-1, 1); err != nil {
						return err
					}
				}

				list.Order = newOrder
			}
		}

		if err := tx.UpdateTodoList(ctx, list); err != nil {
			return err
		}
		if archiveChanged {
			if err := tx.SetCompletedItemsArchived(ctx, list.ID, list.ArchiveCompleted); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return list, nil
}

func (s *Service) DeleteTodoList(ctx context.Context, familyID, listID string) error {
	list, err := s.repo.GetTodoListByID(ctx, familyID, listID)
	if err != nil {
		return err
	}

	return s.repo.Transaction(ctx, func(tx Repository) error {
		if err := tx.SoftDeleteItemsByList(ctx, list.ID); err != nil {
			return err
		}
		deleted, err := tx.SoftDeleteTodoList(ctx, familyID, listID)
		if err != nil {
			return err
		}
		if !deleted {
			return ErrTodoListNotFound
		}
		return nil
	})
}

func (s *Service) ListTodoItems(ctx context.Context, familyID, listID string, archived ArchivedFilter) ([]TodoItem, int64, error) {
	if _, err := s.repo.GetTodoListByID(ctx, familyID, listID); err != nil {
		return nil, 0, err
	}

	items, total, err := s.repo.ListTodoItems(ctx, listID, archived)
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

func (s *Service) CreateTodoItem(ctx context.Context, familyID string, input CreateTodoItemInput) (*TodoItem, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	if _, err := s.repo.GetTodoListByID(ctx, familyID, input.ListID); err != nil {
		return nil, err
	}

	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	item := TodoItem{
		ID:     id,
		ListID: input.ListID,
		Title:  title,
	}

	if err := s.repo.CreateTodoItem(ctx, &item); err != nil {
		return nil, err
	}

	return &item, nil
}

func (s *Service) UpdateTodoItem(ctx context.Context, input UpdateTodoItemInput) (*TodoItem, error) {
	if input.Title == nil && input.IsCompleted == nil {
		return nil, fmt.Errorf("no fields to update")
	}

	item, archiveCompleted, err := s.repo.GetTodoItemWithListArchive(ctx, input.FamilyID, input.ID)
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		if trimmed == "" {
			return nil, fmt.Errorf("title is required")
		}
		item.Title = trimmed
	}

	if input.IsCompleted != nil {
		if *input.IsCompleted {
			if input.CompletedBy == nil || strings.TrimSpace(input.CompletedBy.ID) == "" {
				return nil, fmt.Errorf("completed_by is required")
			}
			now := time.Now().UTC()
			item.IsCompleted = true
			item.CompletedAt = &now
			item.IsArchived = archiveCompleted

			completedByID := strings.TrimSpace(input.CompletedBy.ID)
			completedByName := strings.TrimSpace(input.CompletedBy.Name)
			completedByEmail := strings.TrimSpace(input.CompletedBy.Email)
			completedByAvatar := strings.TrimSpace(input.CompletedBy.AvatarURL)

			item.CompletedByID = &completedByID
			item.CompletedByName = &completedByName
			item.CompletedByEmail = &completedByEmail
			if completedByAvatar == "" {
				item.CompletedByAvatarURL = nil
			} else {
				item.CompletedByAvatarURL = &completedByAvatar
			}
		} else {
			item.IsCompleted = false
			item.IsArchived = false
			item.CompletedAt = nil
			item.CompletedByID = nil
			item.CompletedByName = nil
			item.CompletedByEmail = nil
			item.CompletedByAvatarURL = nil
		}
	}

	if err := s.repo.UpdateTodoItem(ctx, item); err != nil {
		return nil, err
	}

	return item, nil
}

func (s *Service) DeleteTodoItem(ctx context.Context, familyID, itemID string) error {
	item, _, err := s.repo.GetTodoItemWithListArchive(ctx, familyID, itemID)
	if err != nil {
		return err
	}

	deleted, err := s.repo.SoftDeleteTodoItem(ctx, item.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrTodoItemNotFound
	}
	return nil
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
