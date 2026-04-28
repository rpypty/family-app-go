package receipts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	receiptsdomain "family-app-go/internal/domain/receipts"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com"
	defaultOpenAIModel   = "gpt-4o-mini"
	defaultOpenAITimeout = 20 * time.Second
)

type OpenAIParserConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type OpenAIParser struct {
	apiKey     string
	model      string
	baseURL    *url.URL
	httpClient *http.Client
}

func NewOpenAIParser(cfg OpenAIParserConfig) (*OpenAIParser, error) {
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
		model = defaultOpenAIModel
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultOpenAITimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	return &OpenAIParser{
		apiKey:     cfg.APIKey,
		model:      model,
		baseURL:    parsedURL,
		httpClient: httpClient,
	}, nil
}

func (p *OpenAIParser) ParseReceipt(ctx context.Context, input receiptsdomain.ParseReceiptInput) (*receiptsdomain.ParsedReceipt, error) {
	if len(input.Categories) == 0 {
		return nil, receiptsdomain.ErrCategorySelectionRequired
	}
	requestBody, err := p.buildRequest(input)
	if err != nil {
		return nil, err
	}

	endpoint, err := p.resolveURL("/v1/responses")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
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

	var payload parsedReceiptPayload
	if err := json.Unmarshal([]byte(outputText), &payload); err != nil {
		return nil, fmt.Errorf("%w: decode structured output: %v", receiptsdomain.ErrLLMInvalidResponse, err)
	}

	parsed, err := payload.toDomain(input, p.model, rawBody)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", receiptsdomain.ErrLLMInvalidResponse, err)
	}
	return parsed, nil
}

func (p *OpenAIParser) buildRequest(input receiptsdomain.ParseReceiptInput) ([]byte, error) {
	categoryLines := make([]string, 0, len(input.Categories))
	categoryIDs := make([]string, 0, len(input.Categories))
	for _, category := range input.Categories {
		categoryLines = append(categoryLines, fmt.Sprintf("- %s: %s", category.ID, category.Name))
		categoryIDs = append(categoryIDs, category.ID)
	}

	requestedDate := ""
	if input.Date != nil {
		requestedDate = input.Date.Format("2006-01-02")
	}
	requestedCurrency := strings.ToUpper(strings.TrimSpace(input.Currency))

	systemPrompt := strings.Join([]string{
		"You parse a single retail receipt image into expense line items.",
		"Return only JSON that matches the provided schema.",
		"Each item must use one of the allowed category IDs.",
		"Amounts must be positive decimal numbers in receipt currency.",
		"raw_name must preserve the original wording and language from the receipt. Do not translate item names.",
		"normalized_name may clean OCR noise or abbreviations, but it must stay in the same language as the receipt item. Do not translate it.",
		"If a field is unknown, use null for nullable fields and an empty array for warnings.",
	}, "\n")

	userPromptParts := []string{
		fmt.Sprintf(
			"Parse this receipt image.\nAllowed categories:\n%s\nRequested date: %s\nRequested currency: %s\nKeep item names in the original receipt language.\nUse the category IDs exactly as listed.",
			strings.Join(categoryLines, "\n"),
			emptyAsUnknown(requestedDate),
			emptyAsUnknown(requestedCurrency),
		),
	}
	if hintsBlock := buildCorrectionHintsBlock(input.Corrections); hintsBlock != "" {
		userPromptParts = append(userPromptParts, hintsBlock)
	}
	userPrompt := strings.Join(userPromptParts, "\n\n")

	payload := openAIRequest{
		Model: p.model,
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
					{
						Type:     "input_image",
						ImageURL: "data:" + input.File.ContentType + ";base64," + base64.StdEncoding.EncodeToString(input.File.Data),
					},
				},
			},
		},
		Text: openAITextConfig{
			Format: openAITextFormat{
				Type:        "json_schema",
				Name:        "receipt_parse_result",
				Description: "Structured receipt parse result",
				Strict:      true,
				Schema:      buildReceiptSchema(categoryIDs),
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %v", receiptsdomain.ErrLLMRequestFailed, err)
	}
	return raw, nil
}

func (p *OpenAIParser) resolveURL(path string) (string, error) {
	relative, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	return p.baseURL.ResolveReference(relative).String(), nil
}

func buildCorrectionHintsBlock(corrections []receiptsdomain.CorrectionHint) string {
	if len(corrections) == 0 {
		return ""
	}

	lines := []string{"Family-specific category hints:"}
	for _, correction := range corrections {
		name := strings.TrimSpace(correction.CanonicalName)
		categoryName := strings.TrimSpace(correction.CategoryName)
		if name == "" || categoryName == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %q -> %q", name, categoryName))
	}
	if len(lines) == 1 {
		return ""
	}

	lines = append(lines,
		"",
		"Use these as soft hints only.",
		"Do not treat them as strict rules.",
		"If the current receipt item is clearly different, ignore the hint.",
	)
	return strings.Join(lines, "\n")
}

type openAIRequest struct {
	Model string               `json:"model"`
	Input []openAIInputMessage `json:"input"`
	Text  openAITextConfig     `json:"text"`
}

type openAIInputMessage struct {
	Role    string            `json:"role"`
	Content []openAIInputPart `json:"content"`
}

type openAIInputPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type openAITextConfig struct {
	Format openAITextFormat `json:"format"`
}

type openAITextFormat struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Strict      bool           `json:"strict"`
	Schema      map[string]any `json:"schema"`
}

type openAIResponse struct {
	Output []openAIOutputMessage `json:"output"`
}

type openAIOutputMessage struct {
	Type    string                `json:"type"`
	Content []openAIOutputContent `json:"content"`
}

type openAIOutputContent struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

func (r openAIResponse) outputText() (string, error) {
	for _, message := range r.Output {
		for _, content := range message.Content {
			if strings.TrimSpace(content.Refusal) != "" {
				return "", fmt.Errorf("model refused request")
			}
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return content.Text, nil
			}
		}
	}
	return "", fmt.Errorf("response does not contain output_text")
}

type parsedReceiptPayload struct {
	MerchantName  *string                    `json:"merchant_name"`
	PurchasedAt   *string                    `json:"purchased_at"`
	Currency      string                     `json:"currency"`
	DetectedTotal *float64                   `json:"detected_total"`
	Warnings      []string                   `json:"warnings"`
	Items         []parsedReceiptPayloadItem `json:"items"`
}

type parsedReceiptPayloadItem struct {
	RawName            string   `json:"raw_name"`
	NormalizedName     *string  `json:"normalized_name"`
	Quantity           *float64 `json:"quantity"`
	UnitPrice          *float64 `json:"unit_price"`
	LineTotal          float64  `json:"line_total"`
	CategoryID         string   `json:"category_id"`
	CategoryConfidence *float64 `json:"category_confidence"`
}

func (p parsedReceiptPayload) toDomain(input receiptsdomain.ParseReceiptInput, model string, rawResponse []byte) (*receiptsdomain.ParsedReceipt, error) {
	currency := strings.ToUpper(strings.TrimSpace(p.Currency))
	if currency == "" {
		currency = strings.ToUpper(strings.TrimSpace(input.Currency))
	}
	if currency == "" {
		return nil, fmt.Errorf("currency is required")
	}

	allowed := make(map[string]struct{}, len(input.Categories))
	for _, category := range input.Categories {
		allowed[category.ID] = struct{}{}
	}

	var purchasedAt *time.Time
	if p.PurchasedAt != nil && strings.TrimSpace(*p.PurchasedAt) != "" {
		parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(*p.PurchasedAt))
		if err != nil {
			return nil, fmt.Errorf("invalid purchased_at")
		}
		normalized := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC)
		purchasedAt = &normalized
	}

	items := make([]receiptsdomain.ParsedItem, 0, len(p.Items))
	for _, item := range p.Items {
		name := strings.TrimSpace(item.RawName)
		if name == "" {
			return nil, fmt.Errorf("raw_name is required")
		}
		if item.LineTotal <= 0 {
			return nil, fmt.Errorf("line_total must be positive")
		}
		categoryID := strings.TrimSpace(item.CategoryID)
		if _, ok := allowed[categoryID]; !ok {
			return nil, fmt.Errorf("category_id is not allowed")
		}
		if item.Quantity != nil && *item.Quantity <= 0 {
			return nil, fmt.Errorf("quantity must be positive")
		}
		if item.UnitPrice != nil && *item.UnitPrice <= 0 {
			return nil, fmt.Errorf("unit_price must be positive")
		}
		if item.CategoryConfidence != nil && (*item.CategoryConfidence < 0 || *item.CategoryConfidence > 1) {
			return nil, fmt.Errorf("category_confidence must be between 0 and 1")
		}

		categoryIDCopy := categoryID
		items = append(items, receiptsdomain.ParsedItem{
			RawName:            name,
			NormalizedName:     trimStringPtr(item.NormalizedName),
			Quantity:           item.Quantity,
			UnitPrice:          item.UnitPrice,
			LineTotal:          item.LineTotal,
			CategoryID:         &categoryIDCopy,
			CategoryConfidence: item.CategoryConfidence,
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("items are required")
	}
	if p.DetectedTotal != nil && *p.DetectedTotal <= 0 {
		return nil, fmt.Errorf("detected_total must be positive")
	}

	return &receiptsdomain.ParsedReceipt{
		MerchantName:  trimStringPtr(p.MerchantName),
		PurchasedAt:   purchasedAt,
		Currency:      currency,
		DetectedTotal: p.DetectedTotal,
		Warnings:      normalizeWarnings(p.Warnings),
		Provider:      "openai",
		Model:         model,
		RawResponse:   append([]byte(nil), rawResponse...),
		Items:         items,
	}, nil
}

func buildReceiptSchema(categoryIDs []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"merchant_name", "purchased_at", "currency", "detected_total", "warnings", "items"},
		"properties": map[string]any{
			"merchant_name": nullableStringSchema("Merchant name, or null if unknown."),
			"purchased_at":  nullableDateSchema("Receipt purchase date in YYYY-MM-DD format, or null if unknown."),
			"currency": map[string]any{
				"type":        "string",
				"description": "Three-letter ISO currency code for all amounts on the receipt.",
			},
			"detected_total": nullableNumberSchema("Receipt total amount, or null if not confidently detected."),
			"warnings": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []string{
						"raw_name",
						"normalized_name",
						"quantity",
						"unit_price",
						"line_total",
						"category_id",
						"category_confidence",
					},
					"properties": map[string]any{
						"raw_name":            map[string]any{"type": "string", "description": "Original item name copied from the receipt. Preserve the original language and wording. Do not translate."},
						"normalized_name":     nullableStringSchema("Normalized item name in the same language as the receipt item, or null if no normalization is needed. Do not translate."),
						"quantity":            nullablePositiveNumberSchema("Item quantity, or null if unknown."),
						"unit_price":          nullablePositiveNumberSchema("Unit price, or null if unknown."),
						"line_total":          map[string]any{"type": "number", "exclusiveMinimum": 0},
						"category_id":         map[string]any{"type": "string", "enum": categoryIDs},
						"category_confidence": nullableProbabilitySchema("Confidence from 0 to 1, or null if unknown."),
					},
				},
			},
		},
	}
}

func nullableStringSchema(description string) map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "string", "description": description},
			{"type": "null"},
		},
	}
}

func nullableDateSchema(description string) map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "string", "format": "date", "description": description},
			{"type": "null"},
		},
	}
}

func nullableNumberSchema(description string) map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "number", "description": description},
			{"type": "null"},
		},
	}
}

func nullablePositiveNumberSchema(description string) map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "number", "exclusiveMinimum": 0, "description": description},
			{"type": "null"},
		},
	}
}

func nullableProbabilitySchema(description string) map[string]any {
	return map[string]any{
		"anyOf": []map[string]any{
			{"type": "number", "minimum": 0, "maximum": 1, "description": description},
			{"type": "null"},
		},
	}
}

func emptyAsUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func trimStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		trimmed := strings.TrimSpace(warning)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return []string{}
	}
	return result
}
