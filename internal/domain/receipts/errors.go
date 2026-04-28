package receipts

import "errors"

var (
	ErrReceiptParserDisabled       = errors.New("receipt parser disabled")
	ErrActiveReceiptParseExists    = errors.New("active receipt parse exists")
	ErrReceiptParseNotFound        = errors.New("receipt parse not found")
	ErrReceiptParseInvalidStatus   = errors.New("receipt parse invalid status")
	ErrInvalidReceiptFile          = errors.New("invalid receipt file")
	ErrReceiptFileTooLarge         = errors.New("receipt file too large")
	ErrTooManyReceiptFiles         = errors.New("too many receipt files")
	ErrCategorySelectionRequired   = errors.New("category selection required")
	ErrCategoryNotFound            = errors.New("category not found")
	ErrReceiptParseEmpty           = errors.New("receipt parse empty")
	ErrReceiptParseUnresolvedItems = errors.New("receipt parse has unresolved items")
	ErrLLMRequestFailed            = errors.New("llm request failed")
	ErrLLMInvalidResponse          = errors.New("llm invalid response")
)
