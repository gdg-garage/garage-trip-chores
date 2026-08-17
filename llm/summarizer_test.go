package llm

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type mockClient struct {
	lastPrompt string
	response   string
	err        error
	callCount  int
}

func (m *mockClient) GenerateContent(ctx context.Context, prompt string) (string, error) {
	m.callCount++
	m.lastPrompt = prompt
	return m.response, m.err
}

func createTestStorage(t *testing.T) *storage.Storage {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_summarizer.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := storage.New(storage.Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize test storage: %v", err)
	}
	return s
}

func TestSummarizer_NoApiKey(t *testing.T) {
	s := createTestStorage(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mock := &mockClient{response: "Test"}
	summarizer := NewSummarizer(s, nil, logger, Config{ApiKey: ""}, "chan-123")
	summarizer.SetClient(mock)

	err := summarizer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("Expected nil error, got %v", err)
	}
	if mock.callCount != 0 {
		t.Errorf("Expected 0 calls to LLM client when API key is empty, got %d", mock.callCount)
	}
}

func TestSummarizer_NoActivity(t *testing.T) {
	s := createTestStorage(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Initial run record so cutoff is established
	err := s.SaveLLMSummaryLog(&storage.LLMSummaryLog{
		RunAt:  time.Now().Add(-1 * time.Hour),
		Status: "success",
	})
	if err != nil {
		t.Fatalf("Failed to save initial log: %v", err)
	}

	mock := &mockClient{response: "Summary"}
	summarizer := NewSummarizer(s, nil, logger, Config{ApiKey: "test-key"}, "chan-123")
	summarizer.SetClient(mock)

	err = summarizer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("Expected nil error, got %v", err)
	}
	if mock.callCount != 0 {
		t.Errorf("Expected LLM call to be skipped when no activity exists, got %d calls", mock.callCount)
	}
}

func TestSummarizer_WithActivity(t *testing.T) {
	s := createTestStorage(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create initial summary log
	initialTime := time.Now().Add(-2 * time.Hour)
	err := s.SaveLLMSummaryLog(&storage.LLMSummaryLog{
		RunAt:     initialTime,
		StatsJSON: `{"u1": {"worked_min": 10, "worked_count": 1}}`,
		Status:    "success",
	})
	if err != nil {
		t.Fatalf("Failed to save initial log: %v", err)
	}

	// Add new chore & work log
	chore, err := s.SaveChore(storage.Chore{
		Name:             "Wash dishes",
		EstimatedTimeMin: 15,
		Created:          time.Now(),
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	chore.Complete()
	_, err = s.SaveChore(chore)
	if err != nil {
		t.Fatalf("Failed to complete chore: %v", err)
	}

	_, err = s.SaveWorkLog(storage.WorkLog{
		UserId:       "u1",
		ChoreId:      chore.ID,
		TimeSpentMin: 15,
	})
	if err != nil {
		t.Fatalf("Failed to save work log: %v", err)
	}

	mock := &mockClient{response: "🎉 **Dishes are clean!** Alice did amazing work while Bob did nothing!"}
	summarizer := NewSummarizer(s, nil, logger, Config{
		ApiKey:           "test-key",
		DiscordChannelId: "chan-123",
	}, "chan-default")
	summarizer.SetClient(mock)

	err = summarizer.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("Expected nil error, got %v", err)
	}

	if mock.callCount != 1 {
		t.Errorf("Expected 1 call to LLM, got %d", mock.callCount)
	}

	// Verify new log in DB
	lastLog, err := s.GetLastSuccessfulLLMSummaryLog()
	if err != nil {
		t.Fatalf("Failed to get last log: %v", err)
	}
	if lastLog == nil {
		t.Fatal("Expected last log to exist")
	}
	if lastLog.Summary != mock.response {
		t.Errorf("Expected summary %q, got %q", mock.response, lastLog.Summary)
	}
	if lastLog.StatsJSON == "" {
		t.Error("Expected non-empty machine readable stats JSON")
	}
}

func TestSplitMessage(t *testing.T) {
	shortMsg := "This is a short message."
	chunks := splitMessage(shortMsg, 100)
	if len(chunks) != 1 || chunks[0] != shortMsg {
		t.Errorf("Unexpected chunks for short message: %v", chunks)
	}

	longMsg := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	chunks = splitMessage(longMsg, 15)
	if len(chunks) < 2 {
		t.Errorf("Expected at least 2 chunks for long message, got %d", len(chunks))
	}
}
