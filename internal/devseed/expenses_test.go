package devseed

import (
	"context"
	"math/rand"
	"testing"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
)

type fakeExpensesService struct {
	categories []expensesdomain.Category
	expenses   []expensesdomain.CreateExpenseInput
}

func (f *fakeExpensesService) CreateCategory(_ context.Context, input expensesdomain.CreateCategoryInput) (*expensesdomain.Category, error) {
	category := expensesdomain.Category{
		ID:       "00000000-0000-4000-8000-0000000000" + string(rune('a'+len(f.categories))),
		FamilyID: input.FamilyID,
		Name:     input.Name,
		Color:    input.Color,
		Emoji:    input.Emoji,
	}
	f.categories = append(f.categories, category)
	return &category, nil
}

func (f *fakeExpensesService) CreateExpense(_ context.Context, input expensesdomain.CreateExpenseInput) (*expensesdomain.ExpenseWithCategories, error) {
	f.expenses = append(f.expenses, input)
	return &expensesdomain.ExpenseWithCategories{
		Expense: expensesdomain.Expense{
			ID:       "expense-id",
			FamilyID: input.FamilyID,
			UserID:   input.UserID,
			Date:     input.Date,
			Amount:   input.Amount,
			Currency: input.Currency,
			Title:    input.Title,
		},
		CategoryIDs: input.CategoryIDs,
	}, nil
}

func TestExpenseSeederGeneratesRecentRussianCategorizedExpenses(t *testing.T) {
	expenses := &fakeExpensesService{}
	now := func() time.Time {
		return time.Date(2026, 4, 25, 13, 45, 0, 0, time.FixedZone("MSK", 3*60*60))
	}
	seeder := NewExpenseSeederWithClockAndRand(expenses, Config{
		Enabled:          true,
		LookbackMonths:   6,
		MinCategories:    10,
		MaxCategories:    20,
		MaxDailyExpenses: 6,
		Currency:         "USD",
	}, now, rand.New(rand.NewSource(42)))

	result, err := seeder.SeedFamily(context.Background(), SeedFamilyInput{
		FamilyID: "family-1",
		UserID:   "user-1",
		Currency: "BYN",
	})
	if err != nil {
		t.Fatalf("seed family: %v", err)
	}

	if len(expenses.categories) < 10 || len(expenses.categories) > 20 {
		t.Fatalf("expected 10-20 categories, got %d", len(expenses.categories))
	}
	if result.CategoriesCreated != len(expenses.categories) {
		t.Fatalf("expected result categories %d, got %d", len(expenses.categories), result.CategoriesCreated)
	}

	colors := make(map[string]struct{}, len(expenses.categories))
	categoryIDs := make(map[string]struct{}, len(expenses.categories))
	for _, category := range expenses.categories {
		if category.Color == nil || *category.Color == "" {
			t.Fatalf("expected category %q to have color", category.Name)
		}
		if _, ok := colors[*category.Color]; ok {
			t.Fatalf("expected unique category colors, duplicate %s", *category.Color)
		}
		colors[*category.Color] = struct{}{}
		categoryIDs[category.ID] = struct{}{}
	}

	expectedFrom := time.Date(2025, 10, 25, 0, 0, 0, 0, time.UTC)
	expectedTo := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	if !result.From.Equal(expectedFrom) || !result.To.Equal(expectedTo) {
		t.Fatalf("expected date range %s..%s, got %s..%s", expectedFrom, expectedTo, result.From, result.To)
	}

	if len(expenses.expenses) == 0 {
		t.Fatalf("expected expenses to be generated")
	}
	if result.ExpensesCreated != len(expenses.expenses) {
		t.Fatalf("expected result expenses %d, got %d", len(expenses.expenses), result.ExpensesCreated)
	}

	byDay := make(map[time.Time]int)
	for _, expense := range expenses.expenses {
		if expense.Date.Before(expectedFrom) || expense.Date.After(expectedTo) {
			t.Fatalf("expense date %s outside expected range", expense.Date)
		}
		byDay[expense.Date]++
		if expense.FamilyID != "family-1" || expense.UserID != "user-1" {
			t.Fatalf("expense has wrong owner: family=%q user=%q", expense.FamilyID, expense.UserID)
		}
		if expense.Currency != "BYN" || expense.BaseCurrency != "BYN" {
			t.Fatalf("expected input currency BYN to be used, got currency=%q base=%q", expense.Currency, expense.BaseCurrency)
		}
		if len(expense.CategoryIDs) != 1 {
			t.Fatalf("expected exactly one category per expense, got %d", len(expense.CategoryIDs))
		}
		if _, ok := categoryIDs[expense.CategoryIDs[0]]; !ok {
			t.Fatalf("expense references unknown category %q", expense.CategoryIDs[0])
		}
		if !hasCyrillic(expense.Title) {
			t.Fatalf("expected Russian title, got %q", expense.Title)
		}
	}

	for day, count := range byDay {
		if count > 6 {
			t.Fatalf("expected at most 6 expenses per day, got %d on %s", count, day)
		}
	}
}

func TestExpenseSeederDisabled(t *testing.T) {
	expenses := &fakeExpensesService{}
	seeder := NewExpenseSeederWithClockAndRand(expenses, Config{Enabled: false}, time.Now, rand.New(rand.NewSource(1)))

	result, err := seeder.SeedFamily(context.Background(), SeedFamilyInput{
		FamilyID: "family-1",
		UserID:   "user-1",
	})
	if err != nil {
		t.Fatalf("seed family: %v", err)
	}
	if result.CategoriesCreated != 0 || result.ExpensesCreated != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
	if len(expenses.categories) != 0 || len(expenses.expenses) != 0 {
		t.Fatalf("expected no data to be generated")
	}
}

func hasCyrillic(value string) bool {
	for _, r := range value {
		if r >= 'А' && r <= 'я' {
			return true
		}
	}
	return false
}
