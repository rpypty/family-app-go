package app

import (
	"strings"

	"family-app-go/internal/config"
	receiptsdomain "family-app-go/internal/domain/receipts"
	receiptshttp "family-app-go/internal/repository/http/receipts"
	"family-app-go/pkg/logger"
)

func buildReceiptParser(cfg config.ReceiptParserConfig, log logger.Logger) (receiptsdomain.Parser, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "mock"
	}

	if provider != "openai" {
		log.Info("app: using mock receipt parser", "provider", provider)
		return receiptsdomain.NewMockParser(), nil
	}

	if !cfg.Enabled {
		log.Warn("app: receipt parser disabled, using mock parser", "provider", provider)
		return receiptsdomain.NewMockParser(), nil
	}
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
		log.Warn("app: openai api key is empty, using mock receipt parser")
		return receiptsdomain.NewMockParser(), nil
	}

	parser, err := receiptshttp.NewOpenAIParser(receiptshttp.OpenAIParserConfig{
		APIKey:  cfg.OpenAIAPIKey,
		Model:   cfg.OpenAIModel,
		BaseURL: cfg.OpenAIBaseURL,
		Timeout: cfg.OpenAITimeout,
	})
	if err != nil {
		return nil, err
	}

	log.Info("app: using openai receipt parser", "model", cfg.OpenAIModel)
	return parser, nil
}

func buildReceiptHintNormalizer(cfg config.ReceiptParserConfig, log logger.Logger) (receiptsdomain.HintNormalizer, error) {
	if !cfg.HintNormalizerEnabled {
		log.Info("app: receipt hint normalizer disabled, using deterministic fallback")
		return nil, nil
	}
	if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
		log.Warn("app: openai api key is empty, using deterministic receipt hint fallback")
		return nil, nil
	}

	normalizer, err := receiptshttp.NewOpenAIHintNormalizer(receiptshttp.OpenAIHintNormalizerConfig{
		APIKey:  cfg.OpenAIAPIKey,
		Model:   cfg.HintNormalizerModel,
		BaseURL: cfg.OpenAIBaseURL,
		Timeout: cfg.OpenAITimeout,
	})
	if err != nil {
		return nil, err
	}

	log.Info("app: using openai receipt hint normalizer", "model", cfg.HintNormalizerModel)
	return normalizer, nil
}
