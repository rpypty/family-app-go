package receipts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	receiptsdomain "family-app-go/internal/domain/receipts"
)

const defaultOpenAIHintNormalizerModel = "gpt-5.4-nano"

type OpenAIHintNormalizerConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type OpenAIHintNormalizer struct {
	apiKey     string
	model      string
	baseURL    *url.URL
	httpClient *http.Client
}

func NewOpenAIHintNormalizer(cfg OpenAIHintNormalizerConfig) (*OpenAIHintNormalizer, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, receiptsdomain.ErrReceiptParserDisabled
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse openai base url: %w", err)
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultOpenAIHintNormalizerModel
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	return &OpenAIHintNormalizer{
		apiKey:     cfg.APIKey,
		model:      model,
		baseURL:    parsedURL,
		httpClient: httpClient,
	}, nil
}

func (n *OpenAIHintNormalizer) NormalizeCategoryCorrection(ctx context.Context, input receiptsdomain.NormalizeCategoryCorrectionInput) (*receiptsdomain.NormalizeCategoryCorrectionResult, error) {
	requestBody, err := n.buildRequest(input)
	if err != nil {
		return nil, err
	}

	endpoint, err := n.resolveURL("/v1/responses")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+n.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: unexpected status %d", receiptsdomain.ErrLLMRequestFailed, resp.StatusCode)
	}

	var apiResponse openAIResponse
	if err := json.Unmarshal(rawBody, &apiResponse); err != nil {
		return nil, fmt.Errorf("%w: decode response: %v", receiptsdomain.ErrLLMInvalidResponse, err)
	}
	outputText, err := apiResponse.outputText()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMInvalidResponse, err)
	}

	var payload hintNormalizerPayload
	if err := json.Unmarshal([]byte(outputText), &payload); err != nil {
		return nil, fmt.Errorf("%w: decode structured output: %v", receiptsdomain.ErrLLMInvalidResponse, err)
	}
	return payload.toDomain()
}

func (n *OpenAIHintNormalizer) buildRequest(input receiptsdomain.NormalizeCategoryCorrectionInput) ([]byte, error) {
	systemPrompt := strings.Join([]string{
		"You normalize family-specific receipt item category corrections into compact reusable category hints.",
		"Return only JSON that matches the provided schema.",
		"Do not change the corrected category.",
		"Match an existing hint only when the current item is semantically the same product group.",
		"Create a new canonical name when no existing hint is a good match.",
	}, "\n")

	userPrompt := strings.Join([]string{
		"Correction event:",
		fmt.Sprintf("- source item text: %q", input.Event.SourceItemText),
		fmt.Sprintf("- normalized item text: %q", input.Event.NormalizedItemText),
		fmt.Sprintf("- final category: %s (%s)", input.FinalCategory.Name, input.FinalCategory.ID),
		fmt.Sprintf("- LLM category: %s", llmCategoryText(input.LLMCategory)),
		"",
		"Existing hints for this family and final category:",
		existingHintsText(input.ExistingHints),
		"",
		fmt.Sprintf("Use match_existing only if confidence is at least %.2f.", input.ConfidenceCutoff),
	}, "\n")

	payload := openAIRequest{
		Model: n.model,
		Input: []openAIInputMessage{
			{
				Role: "system",
				Content: []openAIInputPart{
					{Type: "input_text", Text: systemPrompt},
				},
			},
			{
				Role: "user",
				Content: []openAIInputPart{
					{Type: "input_text", Text: userPrompt},
				},
			},
		},
		Text: openAITextConfig{
			Format: openAITextFormat{
				Type:        "json_schema",
				Name:        "receipt_hint_normalization_result",
				Description: "Receipt category correction hint normalization decision",
				Strict:      true,
				Schema:      buildHintNormalizerSchema(),
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	return raw, nil
}

func (n *OpenAIHintNormalizer) resolveURL(path string) (string, error) {
	relative, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	return n.baseURL.ResolveReference(relative).String(), nil
}

type hintNormalizerPayload struct {
	Action        string  `json:"action"`
	HintID        *string `json:"hint_id"`
	CanonicalName string  `json:"canonical_name"`
	Confidence    float64 `json:"confidence"`
}

func (p hintNormalizerPayload) toDomain() (*receiptsdomain.NormalizeCategoryCorrectionResult, error) {
	action := receiptsdomain.NormalizeCategoryCorrectionAction(strings.TrimSpace(p.Action))
	switch action {
	case receiptsdomain.NormalizeActionMatchExisting, receiptsdomain.NormalizeActionCreateNew:
	default:
		return nil, fmt.Errorf("%w: invalid normalizer action", receiptsdomain.ErrLLMInvalidResponse)
	}
	if p.Confidence < 0 || p.Confidence > 1 {
		return nil, fmt.Errorf("%w: confidence must be between 0 and 1", receiptsdomain.ErrLLMInvalidResponse)
	}
	return &receiptsdomain.NormalizeCategoryCorrectionResult{
		Action:        action,
		HintID:        trimStringPtr(p.HintID),
		CanonicalName: strings.TrimSpace(p.CanonicalName),
		Confidence:    p.Confidence,
	}, nil
}

func buildHintNormalizerSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"action", "hint_id", "canonical_name", "confidence"},
		"properties": map[string]any{
			"action": map[string]any{
				"type": "string",
				"enum": []string{
					string(receiptsdomain.NormalizeActionMatchExisting),
					string(receiptsdomain.NormalizeActionCreateNew),
				},
			},
			"hint_id": nullableStringSchema("Existing hint ID when action is match_existing, otherwise null."),
			"canonical_name": map[string]any{
				"type":        "string",
				"description": "Compact canonical item group name. Empty when action is match_existing.",
			},
			"confidence": map[string]any{
				"type":    "number",
				"minimum": 0,
				"maximum": 1,
			},
		},
	}
}

func existingHintsText(hints []receiptsdomain.FamilyHint) string {
	if len(hints) == 0 {
		return "- none"
	}
	lines := make([]string, 0, len(hints))
	for _, hint := range hints {
		lines = append(lines, fmt.Sprintf("- %s: %q (%d confirmations)", hint.ID, hint.CanonicalName, hint.TimesConfirmed))
	}
	return strings.Join(lines, "\n")
}

func llmCategoryText(category *receiptsdomain.Category) string {
	if category == nil {
		return "unknown"
	}
	return fmt.Sprintf("%s (%s)", category.Name, category.ID)
}
