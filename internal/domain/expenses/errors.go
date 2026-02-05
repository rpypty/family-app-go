package expenses

import "errors"

var (
	ErrExpenseNotFound = errors.New("expense not found")
	ErrTagNotFound     = errors.New("tag not found")
)
