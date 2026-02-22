package expenses

import "errors"

var (
	ErrExpenseNotFound = errors.New("expense not found")
	ErrTagNotFound     = errors.New("tag not found")
	ErrTagInUse        = errors.New("tag in use")
	ErrTagNameTaken    = errors.New("tag name already exists")
	ErrInvalidTagColor = errors.New("invalid tag color")
	ErrInvalidTagEmoji = errors.New("invalid tag emoji")
)
