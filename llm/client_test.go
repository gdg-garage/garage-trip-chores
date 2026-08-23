package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Test witty chore summary!"}],"role":"model"},"finishReason":"STOP"}]}`))
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

func TestGeminiClient_ServiceTier(t *testing.T) {
	var receivedRequest GenerateContentRequest
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedRequest)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Flex response!"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer ts.Close()

	client := NewGeminiClient(Config{
		ApiKey:      "test-api-key",
		Model:       "gemini-3.7-flash",
		ServiceTier: "flex",
		ApiBaseUrl:  ts.URL,
	})

	output, err := client.GenerateContent(context.Background(), "Prompt")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if output != "Flex response!" {
		t.Errorf("Expected 'Flex response!', got %s", output)
	}
	if receivedRequest.ServiceTier != "flex" {
		t.Errorf("Expected request service_tier 'flex', got %q", receivedRequest.ServiceTier)
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

func TestGeminiClient_FallbackToStandard(t *testing.T) {
	var attempts []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateContentRequest
		json.NewDecoder(r.Body).Decode(&req)
		attempts = append(attempts, req.ServiceTier)

		if req.ServiceTier == "flex" {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"code":503,"message":"High demand"}}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"Standard fallback success!"}],"role":"model"},"finishReason":"STOP"}]}`))
	}))
	defer ts.Close()

	client := NewGeminiClient(Config{
		ApiKey:      "test-api-key",
		Model:       "gemini-3.7-flash",
		ServiceTier: "flex",
		ApiBaseUrl:  ts.URL,
	})
	client.initialBackoff = 1 * time.Millisecond
	client.maxRetries = 2

	output, err := client.GenerateContent(context.Background(), "Prompt")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if output != "Standard fallback success!" {
		t.Errorf("Expected 'Standard fallback success!', got %q", output)
	}

	// Should have tried flex 3 times (1 initial + 2 retries), then tried standard
	flexAttempts := 0
	standardAttempts := 0
	for _, tier := range attempts {
		if tier == "flex" {
			flexAttempts++
		} else if tier == "standard" {
			standardAttempts++
		}
	}

	if flexAttempts != 3 {
		t.Errorf("Expected 3 flex attempts, got %d", flexAttempts)
	}
	if standardAttempts != 1 {
		t.Errorf("Expected 1 standard attempt, got %d", standardAttempts)
	}
}
