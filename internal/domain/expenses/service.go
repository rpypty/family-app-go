package expenses

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	ratesdomain "family-app-go/internal/domain/rates"
)

type Service struct {
	repo            Repository
	categoriesCache CategoriesCache
	rates           RateProvider
}

type RateProvider interface {
	GetRate(ctx context.Context, from, to string, onDate time.Time) (ratesdomain.Quote, error)
}

func NewService(repo Repository) *Service {
	return NewServiceWithDependencies(repo, nil, nil)
}

const categoriesCacheTTL = 60 * time.Second

func NewServiceWithCategoriesCache(repo Repository, categoriesCache CategoriesCache) *Service {
	return NewServiceWithDependencies(repo, categoriesCache, nil)
}

func NewServiceWithDependencies(repo Repository, categoriesCache CategoriesCache, rates RateProvider) *Service {
	if categoriesCache == nil {
		categoriesCache = noopCategoriesCache{}
	}
	return &Service{
		repo:            repo,
		categoriesCache: categoriesCache,
		rates:           rates,
	}
}

func (s *Service) ListExpenses(ctx context.Context, familyID string, filter ListFilter) ([]ExpenseWithCategories, int64, error) {
	expenses, total, err := s.repo.ListExpenses(ctx, familyID, filter)
	if err != nil {
		return nil, 0, err
	}

	if len(expenses) == 0 {
		return []ExpenseWithCategories{}, total, nil
	}

	expenseIDs := make([]string, 0, len(expenses))
	for _, expense := range expenses {
		expenseIDs = append(expenseIDs, expense.ID)
	}

	categoryIDsByExpense, err := s.repo.GetCategoryIDsByExpenseIDs(ctx, expenseIDs)
	if err != nil {
		return nil, 0, err
	}

	items := make([]ExpenseWithCategories, 0, len(expenses))
	for _, expense := range expenses {
		items = append(items, ExpenseWithCategories{
			Expense:     expense,
			CategoryIDs: categoryIDsByExpense[expense.ID],
		})
	}

	return items, total, nil
}

func (s *Service) CreateExpense(ctx context.Context, input CreateExpenseInput) (*ExpenseWithCategories, error) {
	currency, baseCurrency, err := s.validateInput(input.Currency, input.BaseCurrency, input.Title)
	if err != nil {
		return nil, err
	}

	expenseID, err := newUUID()
	if err != nil {
		return nil, err
	}

	expense := Expense{
		ID:       expenseID,
		FamilyID: input.FamilyID,
		UserID:   input.UserID,
		Date:     input.Date,
		Amount:   input.Amount,
		Currency: currency,
		Title:    strings.TrimSpace(input.Title),
	}
	if err := s.applyCurrencyConversion(ctx, &expense, baseCurrency); err != nil {
		return nil, err
	}

	categoryIDs := normalizeCategoryIDs(input.CategoryIDs)
	if err := validateCategoryIDs(categoryIDs); err != nil {
		return nil, err
	}

	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if len(categoryIDs) > 0 {
			count, err := tx.CountCategoriesByIDs(ctx, input.FamilyID, categoryIDs)
			if err != nil {
				return err
			}
			if count != int64(len(categoryIDs)) {
				return ErrCategoryNotFound
			}
		}

		if err := tx.CreateExpense(ctx, &expense); err != nil {
			return err
		}

		return tx.ReplaceExpenseCategories(ctx, expense.ID, categoryIDs)
	})
	if err != nil {
		return nil, err
	}

	return &ExpenseWithCategories{Expense: expense, CategoryIDs: categoryIDs}, nil
}

func (s *Service) UpdateExpense(ctx context.Context, input UpdateExpenseInput) (*ExpenseWithCategories, error) {
	currency, baseCurrency, err := s.validateInput(input.Currency, input.BaseCurrency, input.Title)
	if err != nil {
		return nil, err
	}

	categoryIDs := normalizeCategoryIDs(input.CategoryIDs)
	if err := validateCategoryIDs(categoryIDs); err != nil {
		return nil, err
	}

	var updated Expense
	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if len(categoryIDs) > 0 {
			count, err := tx.CountCategoriesByIDs(ctx, input.FamilyID, categoryIDs)
			if err != nil {
				return err
			}
			if count != int64(len(categoryIDs)) {
				return ErrCategoryNotFound
			}
		}

		expense, err := tx.GetExpenseByID(ctx, input.FamilyID, input.ID)
		if err != nil {
			return err
		}

		expense.Date = input.Date
		expense.Amount = input.Amount
		expense.Currency = currency
		expense.Title = strings.TrimSpace(input.Title)
		expense.UpdatedAt = time.Now().UTC()
		if err := s.applyCurrencyConversion(ctx, expense, baseCurrency); err != nil {
			return err
		}

		if err := tx.UpdateExpense(ctx, expense); err != nil {
			return err
		}

		if err := tx.ReplaceExpenseCategories(ctx, expense.ID, categoryIDs); err != nil {
			return err
		}

		updated = *expense
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ExpenseWithCategories{Expense: updated, CategoryIDs: categoryIDs}, nil
}

func (s *Service) DeleteExpense(ctx context.Context, familyID, expenseID string) error {
	deleted, err := s.repo.DeleteExpense(ctx, familyID, expenseID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrExpenseNotFound
	}
	return nil
}

func (s *Service) ListCategories(ctx context.Context, familyID string) ([]Category, error) {
	if cached, ok := s.categoriesCache.GetByFamilyID(familyID); ok {
		return cloneCategories(cached), nil
	}

	categories, err := s.repo.ListCategories(ctx, familyID)
	if err != nil {
		return nil, err
	}

	s.categoriesCache.SetByFamilyID(familyID, categories, categoriesCacheTTL)
	return cloneCategories(categories), nil
}

func (s *Service) CreateCategory(ctx context.Context, input CreateCategoryInput) (*Category, error) {
	name, err := validateCategoryName(input.Name)
	if err != nil {
		return nil, err
	}

	color, err := normalizeCategoryColor(input.Color)
	if err != nil {
		return nil, err
	}

	emoji, err := normalizeCategoryEmoji(input.Emoji)
	if err != nil {
		return nil, err
	}

	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	category := Category{
		ID:       id,
		FamilyID: input.FamilyID,
		Name:     name,
		Color:    color,
		Emoji:    emoji,
	}

	if err := s.repo.CreateCategory(ctx, &category); err != nil {
		return nil, err
	}

	s.categoriesCache.DeleteByFamilyID(input.FamilyID)
	return &category, nil
}

func (s *Service) UpdateCategory(ctx context.Context, input UpdateCategoryInput) (*Category, error) {
	name, err := validateCategoryName(input.Name)
	if err != nil {
		return nil, err
	}

	category, err := s.repo.GetCategoryByID(ctx, input.FamilyID, input.CategoryID)
	if err != nil {
		return nil, err
	}

	count, err := s.repo.CountCategoriesByName(ctx, input.FamilyID, name, category.ID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrCategoryNameTaken
	}

	category.Name = name
	if input.Color.Set {
		color, err := normalizeCategoryColor(input.Color.Value)
		if err != nil {
			return nil, err
		}
		category.Color = color
	}
	if input.Emoji.Set {
		emoji, err := normalizeCategoryEmoji(input.Emoji.Value)
		if err != nil {
			return nil, err
		}
		category.Emoji = emoji
	}

	if err := s.repo.UpdateCategory(ctx, category); err != nil {
		return nil, err
	}

	s.categoriesCache.DeleteByFamilyID(input.FamilyID)
	return category, nil
}

func (s *Service) DeleteCategory(ctx context.Context, familyID, categoryID string) error {
	inUse, err := s.repo.CountExpenseCategoriesByCategoryID(ctx, categoryID)
	if err != nil {
		return err
	}
	if inUse > 0 {
		return ErrCategoryInUse
	}
	deleted, err := s.repo.DeleteCategory(ctx, familyID, categoryID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrCategoryNotFound
	}
	s.categoriesCache.DeleteByFamilyID(familyID)
	return nil
}

func (s *Service) validateInput(currency, baseCurrency, title string) (string, string, error) {
	if strings.TrimSpace(title) == "" {
		return "", "", fmt.Errorf("title is required")
	}
	normalizedCurrency, err := normalizeCurrencyCode(currency)
	if err != nil {
		return "", "", fmt.Errorf("currency is required")
	}
	normalizedBaseCurrency := normalizedCurrency
	if strings.TrimSpace(baseCurrency) != "" {
		normalizedBaseCurrency, err = normalizeCurrencyCode(baseCurrency)
		if err != nil {
			return "", "", fmt.Errorf("base currency is invalid")
		}
	}

	return normalizedCurrency, normalizedBaseCurrency, nil
}

func (s *Service) applyCurrencyConversion(ctx context.Context, expense *Expense, baseCurrency string) error {
	expense.BaseCurrency = stringPtr(baseCurrency)
	expense.RateDate = timePtr(dateOnlyUTC(expense.Date))

	if expense.Currency == baseCurrency {
		expense.ExchangeRate = float64Ptr(1)
		expense.AmountInBase = float64Ptr(roundMoney(expense.Amount))
		expense.RateSource = stringPtr("identity")
		return nil
	}

	if s.rates == nil {
		return ErrRateNotAvailable
	}

	quote, err := s.rates.GetRate(ctx, expense.Currency, baseCurrency, expense.Date)
	if err != nil {
		if errors.Is(err, ratesdomain.ErrRateNotAvailable) {
			return ErrRateNotAvailable
		}
		return err
	}

	expense.ExchangeRate = float64Ptr(quote.Rate)
	expense.AmountInBase = float64Ptr(roundMoney(expense.Amount * quote.Rate))
	expense.RateDate = timePtr(dateOnlyUTC(quote.Date))
	source := strings.TrimSpace(quote.Source)
	if source == "" {
		source = "unknown"
	}
	expense.RateSource = stringPtr(source)

	return nil
}

func normalizeCurrencyCode(currency string) (string, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if len(currency) != 3 {
		return "", fmt.Errorf("currency must be 3 letters")
	}
	for i := 0; i < len(currency); i++ {
		if currency[i] < 'A' || currency[i] > 'Z' {
			return "", fmt.Errorf("currency must be 3 letters")
		}
	}
	return currency, nil
}

func stringPtr(value string) *string {
	result := value
	return &result
}

func float64Ptr(value float64) *float64 {
	result := value
	return &result
}

func timePtr(value time.Time) *time.Time {
	result := value
	return &result
}

func dateOnlyUTC(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func normalizeCategoryIDs(categoryIDs []string) []string {
	if len(categoryIDs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(categoryIDs))
	result := make([]string, 0, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		value := strings.TrimSpace(categoryID)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}

func validateCategoryIDs(categoryIDs []string) error {
	for _, categoryID := range categoryIDs {
		if !isUUID(categoryID) {
			return ErrCategoryNotFound
		}
	}
	return nil
}

func cloneCategories(categories []Category) []Category {
	if categories == nil {
		return nil
	}
	cloned := make([]Category, len(categories))
	for i := range categories {
		cloned[i] = categories[i]
		if categories[i].Color != nil {
			color := *categories[i].Color
			cloned[i].Color = &color
		}
		if categories[i].Emoji != nil {
			emoji := *categories[i].Emoji
			cloned[i].Emoji = &emoji
		}
	}
	return cloned
}

func validateCategoryName(name string) (string, error) {
	const maxLen = 50
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if len([]rune(name)) > maxLen {
		return "", fmt.Errorf("name must be at most %d characters", maxLen)
	}
	return name, nil
}

var categoryColorRegex = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func normalizeCategoryColor(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	color := strings.ToLower(strings.TrimSpace(*value))
	if !categoryColorRegex.MatchString(color) {
		return nil, ErrInvalidCategoryColor
	}

	return &color, nil
}

func normalizeCategoryEmoji(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	emoji := strings.TrimSpace(*value)
	if emoji == "" {
		return nil, ErrInvalidCategoryEmoji
	}
	if !isSingleEmojiGrapheme(emoji) {
		return nil, ErrInvalidCategoryEmoji
	}

	return &emoji, nil
}

const (
	variationSelector16    rune = 0xFE0F
	zeroWidthJoiner        rune = 0x200D
	combiningEnclosingMark rune = 0x20E3
)

func isSingleEmojiGrapheme(value string) bool {
	runes := []rune(value)
	if len(runes) == 0 {
		return false
	}

	if isKeycapEmoji(runes) {
		return true
	}
	if len(runes) == 2 && isRegionalIndicator(runes[0]) && isRegionalIndicator(runes[1]) {
		return true
	}

	index, ok := consumeEmojiComponent(runes, 0)
	if !ok {
		return false
	}
	for index < len(runes) {
		if runes[index] != zeroWidthJoiner {
			return false
		}
		index++

		next, ok := consumeEmojiComponent(runes, index)
		if !ok {
			return false
		}
		index = next
	}

	return true
}

func consumeEmojiComponent(runes []rune, index int) (int, bool) {
	if index >= len(runes) {
		return index, false
	}
	if !isEmojiBase(runes[index]) {
		return index, false
	}
	index++

	if index < len(runes) && runes[index] == variationSelector16 {
		index++
	}
	if index < len(runes) && isEmojiModifier(runes[index]) {
		index++
	}

	return index, true
}

func isEmojiBase(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	case r >= 0x2300 && r <= 0x23FF:
		return true
	case r >= 0x2B00 && r <= 0x2BFF:
		return true
	case r == 0x00A9 || r == 0x00AE || r == 0x3030 || r == 0x303D || r == 0x3297 || r == 0x3299:
		return true
	}
	return false
}

func isEmojiModifier(r rune) bool {
	return r >= 0x1F3FB && r <= 0x1F3FF
}

func isRegionalIndicator(r rune) bool {
	return r >= 0x1F1E6 && r <= 0x1F1FF
}

func isKeycapEmoji(runes []rune) bool {
	if len(runes) == 2 && isKeycapBase(runes[0]) && runes[1] == combiningEnclosingMark {
		return true
	}
	if len(runes) == 3 && isKeycapBase(runes[0]) && runes[1] == variationSelector16 && runes[2] == combiningEnclosingMark {
		return true
	}
	return false
}

func isKeycapBase(r rune) bool {
	return r == '#' || r == '*' || (r >= '0' && r <= '9')
}

func isUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
			continue
		}
		if !isHex(ch) {
			return false
		}
	}
	return true
}

func isHex(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
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
