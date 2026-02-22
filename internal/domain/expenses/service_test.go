package expenses

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"
)

const tagID1 = "11111111-1111-1111-1111-111111111111"

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
		if len(filter.TagIDs) > 0 {
			if !containsAny(r.expenseTags[expense.ID], filter.TagIDs) {
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

func (r *fakeExpensesRepo) GetTagByID(ctx context.Context, familyID, tagID string) (*Tag, error) {
	tag, ok := r.tags[tagID]
	if !ok || tag.FamilyID != familyID {
		return nil, ErrTagNotFound
	}
	return tag, nil
}

func (r *fakeExpensesRepo) UpdateTag(ctx context.Context, tag *Tag) error {
	if _, ok := r.tags[tag.ID]; !ok {
		return ErrTagNotFound
	}
	r.tags[tag.ID] = tag
	return nil
}

func (r *fakeExpensesRepo) CountTagsByName(ctx context.Context, familyID, name, excludeID string) (int64, error) {
	var count int64
	for _, tag := range r.tags {
		if tag.FamilyID != familyID {
			continue
		}
		if excludeID != "" && tag.ID == excludeID {
			continue
		}
		if strings.EqualFold(tag.Name, name) {
			count++
		}
	}
	return count, nil
}

func (r *fakeExpensesRepo) DeleteTag(ctx context.Context, familyID, tagID string) (bool, error) {
	tag, ok := r.tags[tagID]
	if !ok || tag.FamilyID != familyID {
		return false, nil
	}
	delete(r.tags, tagID)
	return true, nil
}

func (r *fakeExpensesRepo) CountExpenseTagsByTagID(ctx context.Context, tagID string) (int64, error) {
	var count int64
	for _, tags := range r.expenseTags {
		if contains(tags, tagID) {
			count++
		}
	}
	return count, nil
}

func TestCreateExpenseSuccess(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.tags[tagID1] = &Tag{ID: tagID1, FamilyID: "fam-1", Name: "Food"}
	svc := NewService(repo)

	input := CreateExpenseInput{
		FamilyID: "fam-1",
		UserID:   "user-1",
		Date:     time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
		Amount:   12.5,
		Currency: "byn",
		Title:    "Coffee",
		TagIDs:   []string{tagID1},
	}

	result, err := svc.CreateExpense(context.Background(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Currency != "BYN" {
		t.Fatalf("expected currency normalized, got %q", result.Currency)
	}
	if len(result.TagIDs) != 1 || result.TagIDs[0] != tagID1 {
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
		TagIDs:   []string{tagID1},
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
	repo.tags[tagID1] = &Tag{ID: tagID1, FamilyID: "fam-1", Name: "Food"}
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
		TagIDs:   []string{tagID1},
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
	repo.expenseTags["exp-1"] = []string{tagID1}

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
			if len(item.TagIDs) != 1 || item.TagIDs[0] != tagID1 {
				t.Fatalf("expected tags on exp-1, got %v", item.TagIDs)
			}
		}
	}
	if !found {
		t.Fatalf("expected exp-1 in response")
	}
}

func TestListExpensesFilterByTagIDsSingle(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}
	repo.expenseTags["exp-1"] = []string{tagID1}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{TagIDs: []string{tagID1}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != "exp-1" {
		t.Fatalf("expected only exp-1, got %+v", items)
	}
}

func TestListExpensesFilterByTagIDsMultiple(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-3"] = &Expense{ID: "exp-3", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)}
	repo.expenseTags["exp-1"] = []string{tagID1}
	repo.expenseTags["exp-2"] = []string{"22222222-2222-2222-2222-222222222222"}
	repo.expenseTags["exp-3"] = []string{"33333333-3333-3333-3333-333333333333"}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{TagIDs: []string{tagID1, "22222222-2222-2222-2222-222222222222"}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestListExpensesFilterByTagIDsEmptyIgnored(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.expenses["exp-1"] = &Expense{ID: "exp-1", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC)}
	repo.expenses["exp-2"] = &Expense{ID: "exp-2", FamilyID: "fam-1", UserID: "user-1", Date: time.Date(2026, 2, 4, 0, 0, 0, 0, time.UTC)}

	svc := NewService(repo)
	items, total, err := svc.ListExpenses(context.Background(), "fam-1", ListFilter{TagIDs: []string{}})
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

func TestCreateAndDeleteTag(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	created, err := svc.CreateTag(context.Background(), CreateTagInput{
		FamilyID: "fam-1",
		Name:     "Food",
	})
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
	if err := svc.DeleteTag(context.Background(), "fam-1", tagID1); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("expected ErrTagNotFound, got %v", err)
	}
}

func TestCreateTagWithColorAndEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	created, err := svc.CreateTag(context.Background(), CreateTagInput{
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

func TestUpdateTagWithColorAndEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.tags[tagID1] = &Tag{
		ID:       tagID1,
		FamilyID: "fam-1",
		Name:     "Food",
	}
	svc := NewService(repo)

	updated, err := svc.UpdateTag(context.Background(), UpdateTagInput{
		FamilyID: "fam-1",
		TagID:    tagID1,
		Name:     "Food Updated",
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

func TestUpdateTagClearColorAndEmojiWithNull(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.tags[tagID1] = &Tag{
		ID:       tagID1,
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#112233"),
		Emoji:    strPtr("👨‍👩‍👧‍👦"),
	}
	svc := NewService(repo)

	updated, err := svc.UpdateTag(context.Background(), UpdateTagInput{
		FamilyID: "fam-1",
		TagID:    tagID1,
		Name:     "Food",
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

func TestListTagsIncludesColorAndEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	repo.tags[tagID1] = &Tag{
		ID:       tagID1,
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#ffffff"),
		Emoji:    strPtr("🙂"),
	}
	svc := NewService(repo)

	tags, err := svc.ListTags(context.Background(), "fam-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Color == nil || *tags[0].Color != "#ffffff" {
		t.Fatalf("expected color #ffffff, got %+v", tags[0].Color)
	}
	if tags[0].Emoji == nil || *tags[0].Emoji != "🙂" {
		t.Fatalf("expected emoji 🙂, got %+v", tags[0].Emoji)
	}
}

func TestCreateTagInvalidColor(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	_, err := svc.CreateTag(context.Background(), CreateTagInput{
		FamilyID: "fam-1",
		Name:     "Food",
		Color:    strPtr("#GGGGGG"),
	})
	if !errors.Is(err, ErrInvalidTagColor) {
		t.Fatalf("expected ErrInvalidTagColor, got %v", err)
	}
}

func TestCreateTagInvalidEmoji(t *testing.T) {
	repo := newFakeExpensesRepo()
	svc := NewService(repo)

	_, err := svc.CreateTag(context.Background(), CreateTagInput{
		FamilyID: "fam-1",
		Name:     "Food",
		Emoji:    strPtr("ab"),
	})
	if !errors.Is(err, ErrInvalidTagEmoji) {
		t.Fatalf("expected ErrInvalidTagEmoji, got %v", err)
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
