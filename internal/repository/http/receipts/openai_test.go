package receipts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	receiptsdomain "family-app-go/internal/domain/receipts"
)

var testPNGBytes = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d}

func TestOpenAIParserParseReceiptSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Fatalf("unexpected authorization header %q", auth)
		}

		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request["model"] != "gpt-4o-mini" {
			t.Fatalf("unexpected model %#v", request["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{
					"type":"message",
					"content":[
						{
							"type":"output_text",
							"text":"{\"merchant_name\":\"Green\",\"purchased_at\":\"2026-04-26\",\"currency\":\"byn\",\"detected_total\":12.5,\"warnings\":[\"low confidence\"],\"items\":[{\"raw_name\":\"Milk\",\"normalized_name\":\"Milk\",\"quantity\":1,\"unit_price\":12.5,\"line_total\":12.5,\"category_id\":\"cat-1\",\"category_confidence\":0.82}]}"
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	parser, err := NewOpenAIParser(OpenAIParserConfig{
		APIKey:  "test-key",
		Model:   "gpt-4o-mini",
		BaseURL: server.URL,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	parsed, err := parser.ParseReceipt(context.Background(), receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{
			{ID: "cat-1", Name: "Groceries"},
		},
		Currency: "BYN",
	})
	if err != nil {
		t.Fatalf("parse receipt: %v", err)
	}
	if parsed.Provider != "openai" || parsed.Model != "gpt-4o-mini" {
		t.Fatalf("unexpected provider/model %+v", parsed)
	}
	if parsed.Currency != "BYN" {
		t.Fatalf("expected uppercase currency, got %q", parsed.Currency)
	}
	if parsed.MerchantName == nil || *parsed.MerchantName != "Green" {
		t.Fatalf("unexpected merchant name %#v", parsed.MerchantName)
	}
	if len(parsed.Items) != 1 || parsed.Items[0].CategoryID == nil || *parsed.Items[0].CategoryID != "cat-1" {
		t.Fatalf("unexpected items %+v", parsed.Items)
	}
	if len(parsed.Warnings) != 1 || parsed.Warnings[0] != "low confidence" {
		t.Fatalf("unexpected warnings %+v", parsed.Warnings)
	}
}

func TestOpenAIParserBuildRequestPreservesReceiptLanguageInstruction(t *testing.T) {
	parser, err := NewOpenAIParser(OpenAIParserConfig{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	raw, err := parser.buildRequest(receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{
			{ID: "cat-1", Name: "Groceries"},
		},
		Currency: "BYN",
	})
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	var request openAIRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if len(request.Input) < 2 {
		t.Fatalf("unexpected input payload %+v", request.Input)
	}

	systemText := request.Input[0].Content[0].Text
	userText := request.Input[1].Content[0].Text
	if !strings.Contains(systemText, "Do not translate item names") {
		t.Fatalf("missing raw_name language preservation instruction: %q", systemText)
	}
	if !strings.Contains(systemText, "must stay in the same language as the receipt item") {
		t.Fatalf("missing normalized_name language preservation instruction: %q", systemText)
	}
	if !strings.Contains(userText, "Keep item names in the original receipt language") {
		t.Fatalf("missing user language preservation instruction: %q", userText)
	}
}

func TestOpenAIParserParseReceiptInvalidCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{
					"type":"message",
					"content":[
						{
							"type":"output_text",
							"text":"{\"merchant_name\":null,\"purchased_at\":null,\"currency\":\"BYN\",\"detected_total\":10,\"warnings\":[],\"items\":[{\"raw_name\":\"Milk\",\"normalized_name\":null,\"quantity\":null,\"unit_price\":null,\"line_total\":10,\"category_id\":\"cat-2\",\"category_confidence\":0.5}]}"
						}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	parser, err := NewOpenAIParser(OpenAIParserConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	_, err = parser.ParseReceipt(context.Background(), receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{
			{ID: "cat-1", Name: "Groceries"},
		},
		Currency: "BYN",
	})
	if !errors.Is(err, receiptsdomain.ErrLLMInvalidResponse) {
		t.Fatalf("expected ErrLLMInvalidResponse, got %v", err)
	}
}

func TestOpenAIParserParseReceiptRequestFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	parser, err := NewOpenAIParser(OpenAIParserConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	_, err = parser.ParseReceipt(context.Background(), receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{
			{ID: "cat-1", Name: "Groceries"},
		},
		Currency: "BYN",
	})
	if !errors.Is(err, receiptsdomain.ErrLLMRequestFailed) {
		t.Fatalf("expected ErrLLMRequestFailed, got %v", err)
	}
}

func TestOpenAIParserReturnsInvalidResponseOnRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"output":[
				{
					"type":"message",
					"content":[
						{"type":"refusal","refusal":"cannot help"}
					]
				}
			]
		}`))
	}))
	defer server.Close()

	parser, err := NewOpenAIParser(OpenAIParserConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new parser: %v", err)
	}

	_, err = parser.ParseReceipt(context.Background(), receiptsdomain.ParseReceiptInput{
		File: receiptsdomain.UploadedFile{
			FileName:    "receipt.png",
			ContentType: "image/png",
			SizeBytes:   int64(len(testPNGBytes)),
			Data:        testPNGBytes,
		},
		Categories: []receiptsdomain.Category{
			{ID: "cat-1", Name: "Groceries"},
		},
		Currency: "BYN",
	})
	if !errors.Is(err, receiptsdomain.ErrLLMInvalidResponse) {
		t.Fatalf("expected ErrLLMInvalidResponse, got %v", err)
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Fatalf("expected refusal details, got %v", err)
	}
}
