package expenses

import "errors"

var (
	ErrExpenseNotFound      = errors.New("expense not found")
	ErrCategoryNotFound     = errors.New("category not found")
	ErrCategoryInUse        = errors.New("category in use")
	ErrCategoryNameTaken    = errors.New("category name already exists")
	ErrInvalidCategoryColor = errors.New("invalid category color")
	ErrInvalidCategoryEmoji = errors.New("invalid category emoji")
)
