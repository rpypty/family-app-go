package app

import (
	"bytes"
	"testing"
	"time"

	"family-app-go/internal/config"
	receiptsdomain "family-app-go/internal/domain/receipts"
	"family-app-go/pkg/logger"
)

func TestBuildReceiptParserFallsBackToMock(t *testing.T) {
	log := logger.New(&bytes.Buffer{}, logger.LevelCritical, "text")

	parser, err := buildReceiptParser(config.ReceiptParserConfig{
		Provider:      "openai",
		Enabled:       true,
		OpenAITimeout: 2 * time.Second,
	}, log)
	if err != nil {
		t.Fatalf("build receipt parser: %v", err)
	}
	if _, ok := parser.(*receiptsdomain.MockParser); !ok {
		t.Fatalf("expected mock parser fallback, got %T", parser)
	}
}

func TestBuildReceiptParserUsesMockByDefault(t *testing.T) {
	log := logger.New(&bytes.Buffer{}, logger.LevelCritical, "text")

	parser, err := buildReceiptParser(config.ReceiptParserConfig{}, log)
	if err != nil {
		t.Fatalf("build receipt parser: %v", err)
	}
	if _, ok := parser.(*receiptsdomain.MockParser); !ok {
		t.Fatalf("expected mock parser, got %T", parser)
	}
}
