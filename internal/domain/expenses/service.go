package expenses

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) ListExpenses(ctx context.Context, familyID string, filter ListFilter) ([]ExpenseWithTags, int64, error) {
	expenses, total, err := s.repo.ListExpenses(ctx, familyID, filter)
	if err != nil {
		return nil, 0, err
	}

	if len(expenses) == 0 {
		return []ExpenseWithTags{}, total, nil
	}

	expenseIDs := make([]string, 0, len(expenses))
	for _, expense := range expenses {
		expenseIDs = append(expenseIDs, expense.ID)
	}

	tagsByExpense, err := s.repo.GetTagIDsByExpenseIDs(ctx, expenseIDs)
	if err != nil {
		return nil, 0, err
	}

	items := make([]ExpenseWithTags, 0, len(expenses))
	for _, expense := range expenses {
		items = append(items, ExpenseWithTags{
			Expense: expense,
			TagIDs:  tagsByExpense[expense.ID],
		})
	}

	return items, total, nil
}

func (s *Service) CreateExpense(ctx context.Context, input CreateExpenseInput) (*ExpenseWithTags, error) {
	if err := s.validateInput(input.Currency, input.Title); err != nil {
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
		Currency: strings.ToUpper(input.Currency),
		Title:    strings.TrimSpace(input.Title),
	}

	tagIDs := normalizeTagIDs(input.TagIDs)
	if err := validateTagIDs(tagIDs); err != nil {
		return nil, err
	}

	err = s.repo.Transaction(ctx, func(tx Repository) error {
		if len(tagIDs) > 0 {
			count, err := tx.CountTagsByIDs(ctx, input.FamilyID, tagIDs)
			if err != nil {
				return err
			}
			if count != int64(len(tagIDs)) {
				return ErrTagNotFound
			}
		}

		if err := tx.CreateExpense(ctx, &expense); err != nil {
			return err
		}

		return tx.ReplaceExpenseTags(ctx, expense.ID, tagIDs)
	})
	if err != nil {
		return nil, err
	}

	return &ExpenseWithTags{Expense: expense, TagIDs: tagIDs}, nil
}

func (s *Service) UpdateExpense(ctx context.Context, input UpdateExpenseInput) (*ExpenseWithTags, error) {
	if err := s.validateInput(input.Currency, input.Title); err != nil {
		return nil, err
	}

	tagIDs := normalizeTagIDs(input.TagIDs)
	if err := validateTagIDs(tagIDs); err != nil {
		return nil, err
	}

	var updated Expense
	err := s.repo.Transaction(ctx, func(tx Repository) error {
		if len(tagIDs) > 0 {
			count, err := tx.CountTagsByIDs(ctx, input.FamilyID, tagIDs)
			if err != nil {
				return err
			}
			if count != int64(len(tagIDs)) {
				return ErrTagNotFound
			}
		}

		expense, err := tx.GetExpenseByID(ctx, input.FamilyID, input.ID)
		if err != nil {
			return err
		}

		expense.Date = input.Date
		expense.Amount = input.Amount
		expense.Currency = strings.ToUpper(input.Currency)
		expense.Title = strings.TrimSpace(input.Title)
		expense.UpdatedAt = time.Now().UTC()

		if err := tx.UpdateExpense(ctx, expense); err != nil {
			return err
		}

		if err := tx.ReplaceExpenseTags(ctx, expense.ID, tagIDs); err != nil {
			return err
		}

		updated = *expense
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ExpenseWithTags{Expense: updated, TagIDs: tagIDs}, nil
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

func (s *Service) ListTags(ctx context.Context, familyID string) ([]Tag, error) {
	return s.repo.ListTags(ctx, familyID)
}

func (s *Service) CreateTag(ctx context.Context, input CreateTagInput) (*Tag, error) {
	name, err := validateTagName(input.Name)
	if err != nil {
		return nil, err
	}

	color, err := normalizeTagColor(input.Color)
	if err != nil {
		return nil, err
	}

	emoji, err := normalizeTagEmoji(input.Emoji)
	if err != nil {
		return nil, err
	}

	id, err := newUUID()
	if err != nil {
		return nil, err
	}

	tag := Tag{
		ID:       id,
		FamilyID: input.FamilyID,
		Name:     name,
		Color:    color,
		Emoji:    emoji,
	}

	if err := s.repo.CreateTag(ctx, &tag); err != nil {
		return nil, err
	}

	return &tag, nil
}

func (s *Service) UpdateTag(ctx context.Context, input UpdateTagInput) (*Tag, error) {
	name, err := validateTagName(input.Name)
	if err != nil {
		return nil, err
	}

	tag, err := s.repo.GetTagByID(ctx, input.FamilyID, input.TagID)
	if err != nil {
		return nil, err
	}

	count, err := s.repo.CountTagsByName(ctx, input.FamilyID, name, tag.ID)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, ErrTagNameTaken
	}

	tag.Name = name
	if input.Color.Set {
		color, err := normalizeTagColor(input.Color.Value)
		if err != nil {
			return nil, err
		}
		tag.Color = color
	}
	if input.Emoji.Set {
		emoji, err := normalizeTagEmoji(input.Emoji.Value)
		if err != nil {
			return nil, err
		}
		tag.Emoji = emoji
	}

	if err := s.repo.UpdateTag(ctx, tag); err != nil {
		return nil, err
	}

	return tag, nil
}

func (s *Service) DeleteTag(ctx context.Context, familyID, tagID string) error {
	inUse, err := s.repo.CountExpenseTagsByTagID(ctx, tagID)
	if err != nil {
		return err
	}
	if inUse > 0 {
		return ErrTagInUse
	}
	deleted, err := s.repo.DeleteTag(ctx, familyID, tagID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrTagNotFound
	}
	return nil
}

func (s *Service) validateInput(currency, title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(currency) == "" {
		return fmt.Errorf("currency is required")
	}
	return nil
}

func normalizeTagIDs(tagIDs []string) []string {
	if len(tagIDs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(tagIDs))
	result := make([]string, 0, len(tagIDs))
	for _, tagID := range tagIDs {
		value := strings.TrimSpace(tagID)
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

func validateTagIDs(tagIDs []string) error {
	for _, tagID := range tagIDs {
		if !isUUID(tagID) {
			return ErrTagNotFound
		}
	}
	return nil
}

func validateTagName(name string) (string, error) {
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

var tagColorRegex = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func normalizeTagColor(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	color := strings.ToLower(strings.TrimSpace(*value))
	if !tagColorRegex.MatchString(color) {
		return nil, ErrInvalidTagColor
	}

	return &color, nil
}

func normalizeTagEmoji(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}

	emoji := strings.TrimSpace(*value)
	if emoji == "" {
		return nil, ErrInvalidTagEmoji
	}
	if !isSingleEmojiGrapheme(emoji) {
		return nil, ErrInvalidTagEmoji
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
