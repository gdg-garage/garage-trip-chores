package llm

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
)

const (
	DefaultBaseURL     = "https://generativelanguage.googleapis.com/v1beta"
	DefaultModel       = "gemini-3.7-flash"
	DefaultServiceTier = "flex"
)

type Client interface {
	GenerateContent(ctx context.Context, prompt string) (string, error)
}

type GeminiClient struct {
	apiKey         string
	model          string
	baseURL        string
	serviceTier    string
	initialBackoff time.Duration
	maxRetries     int
	httpClient     *http.Client
}

func normalizeModel(m string) string {
	m = strings.TrimSpace(m)
	if m == "" {
		return DefaultModel
	}
	m = strings.TrimPrefix(m, "models/")
	m = strings.ReplaceAll(m, " ", "-")
	return m
}

func NewGeminiClient(conf Config) *GeminiClient {
	baseURL := strings.TrimRight(strings.TrimSpace(conf.ApiBaseUrl), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	model := normalizeModel(conf.Model)
	serviceTier := strings.ToLower(strings.TrimSpace(conf.ServiceTier))
	if serviceTier == "" {
		serviceTier = DefaultServiceTier
	}

	return &GeminiClient{
		apiKey:         strings.TrimSpace(conf.ApiKey),
		model:          model,
		baseURL:        baseURL,
		serviceTier:    serviceTier,
		initialBackoff: 10 * time.Second,
		maxRetries:     5,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

type GenerateContentRequest struct {
	Contents         []Content         `json:"contents"`
	GenerationConfig *GenerationConfig `json:"generationConfig,omitempty"`
	ServiceTier      string            `json:"service_tier,omitempty"`
}

type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

type Part struct {
	Text string `json:"text"`
}

type GenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type GenerateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (c *GeminiClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("gemini API key is empty")
	}

	tiers := []string{c.serviceTier}
	if c.serviceTier == "flex" {
		// Fallback to standard tier if flex tier fails
		tiers = append(tiers, "standard")
	}

	var lastErr error
	for _, tier := range tiers {
		res, err := c.generateContentWithTier(ctx, prompt, tier)
		if err == nil {
			return res, nil
		}
		lastErr = err
	}

	return "", lastErr
}

func (c *GeminiClient) generateContentWithTier(ctx context.Context, prompt string, tier string) (string, error) {
	reqBody := GenerateContentRequest{
		Contents: []Content{
			{
				Role: "user",
				Parts: []Part{
					{Text: prompt},
				},
			},
		},
		GenerationConfig: &GenerationConfig{
			Temperature:     0.8,
			MaxOutputTokens: 1200,
		},
		ServiceTier: tier,
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	endpointURL := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, url.PathEscape(c.model), url.QueryEscape(c.apiKey))

	maxRetries := c.maxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	baseBackoff := c.initialBackoff
	if baseBackoff <= 0 {
		baseBackoff = 10 * time.Second
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 10s, 20s, 40s, 60s, 60s (capped at 60s)
			backoff := baseBackoff * (1 << (attempt - 1))
			if backoff > 60*time.Second {
				backoff = 60 * time.Second
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(reqBytes))
		if err != nil {
			return "", fmt.Errorf("failed to create http request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("gemini API request failed (tier=%s): %w", tier, err)
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response body (tier=%s): %w", tier, err)
			continue
		}

		// Retry on temporary server errors (503 Service Unavailable, 429 Rate Limit)
		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("gemini API returned retryable status %d (tier=%s): %s", resp.StatusCode, tier, string(respBytes))
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("gemini API returned status %d (tier=%s): %s", resp.StatusCode, tier, string(respBytes))
		}

		var genResp GenerateContentResponse
		if err := json.Unmarshal(respBytes, &genResp); err != nil {
			return "", fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if genResp.Error != nil {
			return "", fmt.Errorf("gemini API error %d: %s", genResp.Error.Code, genResp.Error.Message)
		}

		if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("gemini API returned empty candidate response")
		}

		var sb strings.Builder
		for _, part := range genResp.Candidates[0].Content.Parts {
			sb.WriteString(part.Text)
		}

		return sb.String(), nil
	}

	return "", fmt.Errorf("gemini API call failed after %d retries (tier=%s): %w", maxRetries, tier, lastErr)
}
