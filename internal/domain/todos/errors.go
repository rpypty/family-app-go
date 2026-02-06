package todos

import "errors"

var (
	ErrTodoListNotFound = errors.New("todo list not found")
	ErrTodoItemNotFound = errors.New("todo item not found")
)
