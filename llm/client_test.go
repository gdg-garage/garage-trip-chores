package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizeModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "gemini-3.7-flash"},
		{"models/gemini-3.7-flash", "gemini-3.7-flash"},
		{"gemini 3.7 flash", "gemini-3.7-flash"},
		{"gemini-2.0-flash", "gemini-2.0-flash"},
		{"  gemini-2.5-pro  ", "gemini-2.5-pro"},
	}

	for _, tt := range tests {
		got := normalizeModel(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeModel(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestGeminiClient_GenerateContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}
		if r.URL.Query().Get("key") != "test-api-key" {
			t.Errorf("Expected key 'test-api-key', got %s", r.URL.Query().Get("key"))
		}

		resp := GenerateContentResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
					Role string `json:"role"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
						Role string `json:"role"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{Text: "Test witty chore summary!"},
						},
						Role: "model",
					},
					FinishReason: "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewGeminiClient(Config{
		ApiKey:     "test-api-key",
		Model:      "gemini-3.7-flash",
		ApiBaseUrl: ts.URL,
	})

	output, err := client.GenerateContent(context.Background(), "Hello test prompt")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	expected := "Test witty chore summary!"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}
}

func TestGeminiClient_EmptyApiKey(t *testing.T) {
	client := NewGeminiClient(Config{
		ApiKey: "",
	})

	_, err := client.GenerateContent(context.Background(), "Test")
	if err == nil {
		t.Error("Expected error for empty API key, got nil")
	}
}
