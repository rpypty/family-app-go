package expenses

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"
)

type fakeExpensesRepo struct {
	expenses    map[string]*Expense
	tags        map[string]*Tag
	expenseTags map[string][]string
}

func newFakeExpensesRepo() *fakeExpensesRepo {
	return &fakeExpensesRepo{
		expenses:    make(map[string]*Expense),
		tags:        make(map[string]*Tag),
		expenseTags: make(map[string][]string),
	}
}

func (r *fakeExpensesRepo) Transaction(ctx context.Context, fn func(Repository) error) error {
	return fn(r)
}

func (r *fakeExpensesRepo) ListExpenses(ctx context.Context, familyID string, filter ListFilter) ([]Expense, int64, error) {
	items := make([]Expense, 0)
	for _, expense := range r.expenses {
		if expense.FamilyID != familyID {
			continue
		}
		if filter.From != nil && expense.Date.Before(*filter.From) {
			continue
		}
		if filter.To != nil && expense.Date.After(*filter.To) {
			continue
		}
		if filter.TagID != "" {
			if !contains(r.expenseTags[expense.ID], filter.TagID) {
				continue
			}
		}
		items = append(items, *expense)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	total := int64(len(items))
	if filter.Offset > 0 {
		if filter.Offset >= len(items) {
			return []Expense{}, total, nil
		}
		items = items[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(items) {
		items = items[:filter.Limit]
	}

	return items, total, nil
}

func (r *fakeExpensesRepo) GetExpenseByID(ctx context.Context, familyID, expenseID string) (*Expense, error) {
	expense, ok := r.expenses[expenseID]
	if !ok || expense.FamilyID != familyID {
		return nil, ErrExpenseNotFound
	}
	return expense, nil
}

func (r *fakeExpensesRepo) CreateExpense(ctx context.Context, expense *Expense) error {
	r.expenses[expense.ID] = expense
	return nil
}

func (r *fakeExpensesRepo) UpdateExpense(ctx context.Context, expense *Expense) error {
	if _, ok := r.expenses[expense.ID]; !ok {
		return ErrExpenseNotFound
	}
	r.expenses[expense.ID] = expense
	return nil
}

func (r *fakeExpensesRepo) DeleteExpense(ctx context.Context, familyID, expenseID string) (bool, error) {
	expense, ok := r.expenses[expenseID]
	if !ok || expense.FamilyID != familyID {
		return false, nil
	}
	delete(r.expenses, expenseID)
	delete(r.expenseTags, expenseID)
	return true, nil
}

func (r *fakeExpensesRepo) ReplaceExpenseTags(ctx context.Context, expenseID string, tagIDs []string) error {
	r.expenseTags[expenseID] = append([]string{}, tagIDs...)
	return nil
}

func (r *fakeExpensesRepo) GetTagIDsByExpenseIDs(ctx context.Context, expenseIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(expenseIDs))
	for _, id := range expenseIDs {
		result[id] = append([]string{}, r.expenseTags[id]...)
	}
	return result, nil
}

func (r *fakeExpensesRepo) CountTagsByIDs(ctx context.Context, familyID string, tagIDs []string) (int64, error) {
	var count int64
	seen := make(map[string]struct{}, len(tagIDs))
	for _, tagID := range tagIDs {
		if _, ok := seen[tagID]; ok {
			continue
		}
		seen[tagID] = struct{}{}
		if tag, ok := r.tags[tagID]; ok && tag.FamilyID == familyID {
			count++
		}
	}
	return count, nil
}

func (r *fakeExpensesRepo) ListTags(ctx context.Context, familyID string) ([]Tag, error) {
	result := make([]Tag, 0)
	for _, tag := range r.tags {
		if tag.FamilyID == familyID {
			result = append(result, *tag)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (r *fakeExpensesRepo) CreateTag(ctx context.Context, tag *Tag) error {
	r.tags[tag.ID] = tag
	return nil
}

func (r *fakeExpensesRepo) DeleteTag(ctx context.Context, familyID, tagID string) (bool, error) {
	tag, ok := r.tags[tagID]
	if !ok || tag.FamilyID != familyID {
		return false, nil
	}
	delete(r.tags, tagID)
	return true, nil
}

func TestCreateExpenseSuccess(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.tags["tag-1"] = &Tag{ID: "tag-1", FamilyID: "fam-1", Name: "Food"}
	svc := NewService(repo)

	input := CreateExpenseInput{
		FamilyID: "fam-1",
		UserID:   "user-1",
		Date:     time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:   12.5,
		Currency: "byn",
		Title:    "Coffee",
		TagIDs:   []string{"tag-1"},
	}

	result, err := svc.CreateExpense(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Currency != "BYN" {
		t.Fatalf("expected currency normalized, got %q", result.Currency)
	}
	if len(result.TagIDs) != 1 || result.TagIDs[0] != "tag-1" {
		t.Fatalf("expected tag ids, got %+v", result.TagIDs)
	}
	if repo.expenses[result.ID] == nil {
		t.Fatalf("expense not stored")
	}
}

func TestCreateExpenseTagNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	input := CreateExpenseInput{
		FamilyID: "fam-1",
		UserID:   "user-1",
		Date:     time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:   12.5,
		Currency: "BYN",
		Title:    "Coffee",
		TagIDs:   []string{"tag-1"},
	}

	_, err := svc.CreateExpense(context.Background(), input)
	if !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound, got %v", err)
	}
}

func TestUpdateExpenseNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	input := UpdateExpenseInput{
		ID:       "exp-1",
		FamilyID: "fam-1",
		Date:     time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:   10,
		Currency: "BYN",
		Title:    "Coffee",
		TagIDs:   nil,
	}

	_, err := svc.UpdateExpense(context.Background(), input)
	if !errors.Is(err, ErrExpenseNotFound) {
		t.Fatalf("expected ErrExpenseNotFound, got %v", err)
	}
}

func TestUpdateExpenseSuccess(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.tags["tag-1"] = &Tag{ID: "tag-1", FamilyID: "fam-1", Name: "Food"}
	repo.expenses["exp-1"] = &Expense{
		ID:       "exp-1",
		FamilyID: "fam-1",
		UserID:   "user-1",
		Date:     time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		Amount:   5,
		Currency: "BYN",
		Title:    "Old",
	}

	svc := NewService(repo)
	input := UpdateExpenseInput{
		ID:       "exp-1",
		FamilyID: "fam-1",
		Date:     time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:   10,
		Currency: "usd",
		Title:    "New",
		TagIDs:   []string{"tag-1"},
	}

	result, err := svc.UpdateExpense(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Currency != "USD" {
		t.Fatalf("expected currency normalized, got %q", result.Currency)
	}
	if result.Title != "New" {
		t.Fatalf("expected updated title, got %q", result.Title)
	}
}

func TestListExpensesMergesTags(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}
	repo.expenseTags["exp-1"] = []string{"tag-1"}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	found := false
	for _, item := range items {
		if item.ID == "exp-1" {
			found = true
			if len(item.TagIDs) != 1 || item.TagIDs[0] != "tag-1" {
				t.Fatalf("expected tags on exp-1, got %v", item.TagIDs)
			}
		}
	}
	if !found {
		t.Fatalf("expected exp-1 in response")
	}
}

func TestDeleteExpenseNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)
	if err := svc.DeleteExpense(context.Background(), "fam-1", "exp-1"); !errors.Is(err, ErrExpenseNotFound) {
		t.Fatalf("expected ErrExpenseNotFound, got %v", err)
	}
}

func TestCreateAndDeleteTag(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	created, err := svc.CreateTag(context.Background(), "fam-1", "Food")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.Name != "Food" {
		t.Fatalf("expected tag name, got %q", created.Name)
	}

	if err := svc.DeleteTag(context.Background(), "fam-1", created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := repo.tags[created.ID]; ok {
		t.Fatalf("expected tag deleted")
	}
}

func TestDeleteTagNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)
	if err := svc.DeleteTag(context.Background(), "fam-1", "tag-1"); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound, got %v", err)
	}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
