package receipts

import (
	"context"
	"encoding/json"
	"time"
)

type MockParser struct{}

func NewMockParser() *MockParser {
	return &MockParser{}
}

func (p *MockParser) ParseReceipt(_ context.Context, input ParseReceiptInput) (*ParsedReceipt, error) {
	if len(input.Categories) == 0 {
		return nil, ErrCategorySelectionRequired
	}
	files := parseReceiptFiles(input)
	if err := validateUploadedFiles(files); err != nil {
		return nil, err
	}

	category := input.Categories[0]
	confidence := 0.7
	total := 10.00
	name := "Receipt item"
	purchasedAt := input.Date
	if purchasedAt == nil {
		now := time.Now().UTC()
		parsed := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		purchasedAt = &parsed
	}
	currency := input.Currency
	if currency == "" {
		currency = "BYN"
	}
	raw, _ := json.Marshal(map[string]any{
		"mock":  true,
		"files": len(files),
		"file":  files[0].FileName,
	})

	return &ParsedReceipt{
		PurchasedAt:   purchasedAt,
		Currency:      currency,
		DetectedTotal: &total,
		Provider:      "mock",
		Model:         "mock-receipt-parser",
		RawResponse:   raw,
		Items: []ParsedItem{
			{
				RawName:            name,
				NormalizedName:     &name,
				LineTotal:          total,
				CategoryID:         &category.ID,
				CategoryConfidence: &confidence,
			},
		},
		Warnings: []string{},
	}, nil
}
