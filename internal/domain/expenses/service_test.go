package expenses

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

const categoryID1 = "11111111-1111-1111-1111-111111111111"

type fakeExpensesRepo struct {
	expenses          map[string]*Expense
	categories        map[string]*Category
	expenseCategories map[string][]string
}

func newFakeExpensesRepo() *fakeExpensesRepo {
	return &fakeExpensesRepo{
		expenses:          make(map[string]*Expense),
		categories:        make(map[string]*Category),
		expenseCategories: make(map[string][]string),
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
		if len(filter.CategoryIDs) > 0 {
			if !containsAny(r.expenseCategories[expense.ID], filter.CategoryIDs) {
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
	delete(r.expenseCategories, expenseID)
	return true, nil
}

func (r *fakeExpensesRepo) ReplaceExpenseCategories(ctx context.Context, expenseID string, categoryIDs []string) error {
	r.expenseCategories[expenseID] = append([]string{}, categoryIDs...)
	return nil
}

func (r *fakeExpensesRepo) GetCategoryIDsByExpenseIDs(ctx context.Context, expenseIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(expenseIDs))
	for _, id := range expenseIDs {
		result[id] = append([]string{}, r.expenseCategories[id]...)
	}
	return result, nil
}

func (r *fakeExpensesRepo) CountCategoriesByIDs(ctx context.Context, familyID string, categoryIDs []string) (int64, error) {
	var count int64
	seen := make(map[string]struct{}, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		if _, ok := seen[categoryID]; ok {
			continue
		}
		seen[categoryID] = struct{}{}
		if category, ok := r.categories[categoryID]; ok && category.FamilyID == familyID {
			count++
		}
	}
	return count, nil
}

func (r *fakeExpensesRepo) ListCategories(ctx context.Context, familyID string) ([]Category, error) {
	result := make([]Category, 0)
	for _, category := range r.categories {
		if category.FamilyID == familyID {
			result = append(result, *category)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (r *fakeExpensesRepo) CreateCategory(ctx context.Context, category *Category) error {
	r.categories[category.ID] = category
	return nil
}

func (r *fakeExpensesRepo) GetCategoryByID(ctx context.Context, familyID, categoryID string) (*Category, error) {
	category, ok := r.categories[categoryID]
	if !ok || category.FamilyID != familyID {
		return nil, ErrCategoryNotFound
	}
	return category, nil
}

func (r *fakeExpensesRepo) UpdateCategory(ctx context.Context, category *Category) error {
	if _, ok := r.categories[category.ID]; !ok {
		return ErrCategoryNotFound
	}
	r.categories[category.ID] = category
	return nil
}

func (r *fakeExpensesRepo) CountCategoriesByName(ctx context.Context, familyID, name, excludeID string) (int64, error) {
	var count int64
	for _, category := range r.categories {
		if category.FamilyID != familyID {
			continue
		}
		if excludeID != "" && category.ID == excludeID {
			continue
		}
		if strings.EqualFold(category.Name, name) {
			count++
		}
	}
	return count, nil
}

func (r *fakeExpensesRepo) DeleteCategory(ctx context.Context, familyID, categoryID string) (bool, error) {
	category, ok := r.categories[categoryID]
	if !ok || category.FamilyID != familyID {
		return false, nil
	}
	delete(r.categories, categoryID)
	return true, nil
}

func (r *fakeExpensesRepo) CountExpenseCategoriesByCategoryID(ctx context.Context, categoryID string) (int64, error) {
	var count int64
	for _, categories := range r.expenseCategories {
		if contains(categories, categoryID) {
			count++
		}
	}
	return count, nil
}

func TestCreateExpenseSuccess(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.categories[categoryID1] = &Category{ID: categoryID1, FamilyID: "fam-1", Name: "Food"}
	svc := NewService(repo)

	input := CreateExpenseInput{
		FamilyID:    "fam-1",
		UserID:      "user-1",
		Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:      12.5,
		Currency:    "byn",
		Title:       "Coffee",
		CategoryIDs: []string{categoryID1},
	}

	result, err := svc.CreateExpense(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Currency != "BYN" {
		t.Fatalf("expected currency normalized, got %q", result.Currency)
	}
	if len(result.CategoryIDs) != 1 || result.CategoryIDs[0] != categoryID1 {
		t.Fatalf("expected category ids, got %+v", result.CategoryIDs)
	}
	if repo.expenses[result.ID] == nil {
		t.Fatalf("expense not stored")
	}
}

func TestCreateExpenseCategoryNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	input := CreateExpenseInput{
		FamilyID:    "fam-1",
		UserID:      "user-1",
		Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:      12.5,
		Currency:    "BYN",
		Title:       "Coffee",
		CategoryIDs: []string{categoryID1},
	}

	_, err := svc.CreateExpense(context.Background(), input)
	if !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got %v", err)
	}
}

func TestUpdateExpenseNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	input := UpdateExpenseInput{
		ID:          "exp-1",
		FamilyID:    "fam-1",
		Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:      10,
		Currency:    "BYN",
		Title:       "Coffee",
		CategoryIDs: nil,
	}

	_, err := svc.UpdateExpense(context.Background(), input)
	if !errors.Is(err, ErrExpenseNotFound) {
		t.Fatalf("expected ErrExpenseNotFound, got %v", err)
	}
}

func TestUpdateExpenseSuccess(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.categories[categoryID1] = &Category{ID: categoryID1, FamilyID: "fam-1", Name: "Food"}
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
		ID:          "exp-1",
		FamilyID:    "fam-1",
		Date:        time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:      10,
		Currency:    "usd",
		Title:       "New",
		CategoryIDs: []string{categoryID1},
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

func TestListExpensesMergesCategories(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}
	repo.expenseCategories["exp-1"] = []string{categoryID1}

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
			if len(item.CategoryIDs) != 1 || item.CategoryIDs[0] != categoryID1 {
				t.Fatalf("expected categories on exp-1, got %v", item.CategoryIDs)
			}
		}
	}
	if !found {
		t.Fatalf("expected exp-1 in response")
	}
}

func TestListExpensesFilterByCategoryIDsSingle(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}
	repo.expenseCategories["exp-1"] = []string{categoryID1}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{CategoryIDs: []string{categoryID1}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "exp-1" {
		t.Fatalf("expected only exp-1, got %+v", items)
	}
}

func TestListExpensesFilterByCategoryIDsMultiple(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-3"] = &Expense{ID: "exp-3", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)}
	repo.expenseCategories["exp-1"] = []string{categoryID1}
	repo.expenseCategories["exp-2"] = []string{"22222222-2222-2222-2222-222222222222"}
	repo.expenseCategories["exp-3"] = []string{"33333333-3333-3333-3333-333333333333"}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{CategoryIDs: []string{categoryID1, "22222222-2222-2222-2222-222222222222"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestListExpensesFilterByCategoryIDsEmptyIgnored(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{CategoryIDs: []string{}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestDeleteExpenseNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)
	if err := svc.DeleteExpense(context.Background(), "fam-1", "exp-1"); !errors.Is(err, ErrExpenseNotFound) {
		t.Fatalf("expected ErrExpenseNotFound, got %v", err)
	}
}

func TestCreateAndDeleteCategory(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	created, err := svc.CreateCategory(context.Background(), CreateCategoryInput{
		FamilyID: "fam-1",
		Name:     "Food",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.Name != "Food" {
		t.Fatalf("expected category name, got %q", created.Name)
	}

	if err := svc.DeleteCategory(context.Background(), "fam-1", created.ID); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if _, ok := repo.categories[created.ID]; ok {
		t.Fatalf("expected category deleted")
	}
}

func TestDeleteCategoryNotFound(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)
	if err := svc.DeleteCategory(context.Background(), "fam-1", categoryID1); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("expected ErrCategoryNotFound, got %v", err)
	}
}

func TestCreateCategoryWithColorAndEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	created, err := svc.CreateCategory(context.Background(), CreateCategoryInput{
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#A1B2C3"),
		Emoji:    strPtr("👨‍👩‍👧‍👦"),
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if created.Color == nil || *created.Color != "#a1b2c3" {
		t.Fatalf("expected normalized color, got %+v", created.Color)
	}
	if created.Emoji == nil || *created.Emoji != "👨‍👩‍👧‍👦" {
		t.Fatalf("expected emoji, got %+v", created.Emoji)
	}
}

func TestUpdateCategoryWithColorAndEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.categories[categoryID1] = &Category{
		ID:       categoryID1,
		FamilyID: "fam-1",
		Name:     "Food",
	}
	svc := NewService(repo)

	updated, err := svc.UpdateCategory(context.Background(), UpdateCategoryInput{
		FamilyID:   "fam-1",
		CategoryID: categoryID1,
		Name:       "Food Updated",
		Color: OptionalNullableString{
			Set:   true,
			Value: strPtr("#00FFAA"),
		},
		Emoji: OptionalNullableString{
			Set:   true,
			Value: strPtr("❤️"),
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Name != "Food Updated" {
		t.Fatalf("expected updated name, got %q", updated.Name)
	}
	if updated.Color == nil || *updated.Color != "#00ffaa" {
		t.Fatalf("expected normalized color, got %+v", updated.Color)
	}
	if updated.Emoji == nil || *updated.Emoji != "❤️" {
		t.Fatalf("expected emoji, got %+v", updated.Emoji)
	}
}

func TestUpdateCategoryClearColorAndEmojiWithNull(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.categories[categoryID1] = &Category{
		ID:       categoryID1,
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#112233"),
		Emoji:    strPtr("👨‍👩‍👧‍👦"),
	}
	svc := NewService(repo)

	updated, err := svc.UpdateCategory(context.Background(), UpdateCategoryInput{
		FamilyID:   "fam-1",
		CategoryID: categoryID1,
		Name:       "Food",
		Color: OptionalNullableString{
			Set:   true,
			Value: nil,
		},
		Emoji: OptionalNullableString{
			Set:   true,
			Value: nil,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if updated.Color != nil {
		t.Fatalf("expected nil color, got %+v", updated.Color)
	}
	if updated.Emoji != nil {
		t.Fatalf("expected nil emoji, got %+v", updated.Emoji)
	}
}

func TestListCategoriesIncludesColorAndEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.categories[categoryID1] = &Category{
		ID:       categoryID1,
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#ffffff"),
		Emoji:    strPtr("🙂"),
	}
	svc := NewService(repo)

	categories, err := svc.ListCategories(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(categories))
	}
	if categories[0].Color == nil || *categories[0].Color != "#ffffff" {
		t.Fatalf("expected color #ffffff, got %+v", categories[0].Color)
	}
	if categories[0].Emoji == nil || *categories[0].Emoji != "🙂" {
		t.Fatalf("expected emoji 🙂, got %+v", categories[0].Emoji)
	}
}

func TestCreateCategoryInvalidColor(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	_, err := svc.CreateCategory(context.Background(), CreateCategoryInput{
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#GGGGGG"),
	})
	if !errors.Is(err, ErrInvalidCategoryColor) {
		t.Fatalf("expected ErrInvalidCategoryColor, got %v", err)
	}
}

func TestCreateCategoryInvalidEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	_, err := svc.CreateCategory(context.Background(), CreateCategoryInput{
		FamilyID: "fam-1",
		Name:     "Food",
		Emoji:    strPtr("ab"),
	})
	if !errors.Is(err, ErrInvalidCategoryEmoji) {
		t.Fatalf("expected ErrInvalidCategoryEmoji, got %v", err)
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

func containsAny(values []string, candidates []string) bool {
	for _, candidate := range candidates {
		if contains(values, candidate) {
			return true
		}
	}
	return false
}

func strPtr(value string) *string {
	return &value
}
