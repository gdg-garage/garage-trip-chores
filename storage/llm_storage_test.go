package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createTestStorage(t *testing.T) *Storage {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := New(Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize test storage: %v", err)
	}
	return s
}

func TestLLMSummaryLog(t *testing.T) {
	s := createTestStorage(t)

	// Initially should be nil
	lastLog, err := s.GetLastSuccessfulLLMSummaryLog()
	if err != nil {
		t.Fatalf("Unexpected error getting last log: %v", err)
	}
	if lastLog != nil {
		t.Fatalf("Expected nil log, got %v", lastLog)
	}

	// Save a skipped log and a successful log
	now := time.Now()
	err = s.SaveLLMSummaryLog(&LLMSummaryLog{
		RunAt:      now.Add(-2 * time.Hour),
		StatsJSON:  `{"user1": {"worked_min": 10}}`,
		Summary:    "Skipped due to no tasks",
		TasksCount: 0,
		Status:     "skipped_no_activity",
	})
	if err != nil {
		t.Fatalf("Failed to save skipped log: %v", err)
	}

	err = s.SaveLLMSummaryLog(&LLMSummaryLog{
		RunAt:      now.Add(-1 * time.Hour),
		StatsJSON:  `{"user1": {"worked_min": 20}}`,
		Summary:    "Great work by everyone!",
		TasksCount: 3,
		Status:     "success",
	})
	if err != nil {
		t.Fatalf("Failed to save successful log: %v", err)
	}

	lastLog, err = s.GetLastSuccessfulLLMSummaryLog()
	if err != nil {
		t.Fatalf("Failed to get last successful log: %v", err)
	}
	if lastLog == nil {
		t.Fatal("Expected last log to be non-nil")
	}
	if lastLog.Status != "success" {
		t.Errorf("Expected status 'success', got %s", lastLog.Status)
	}
	if lastLog.TasksCount != 3 {
		t.Errorf("Expected TasksCount 3, got %d", lastLog.TasksCount)
	}
}

func TestGetTasksActivitySince(t *testing.T) {
	s := createTestStorage(t)

	baseTime := time.Now().Add(-10 * time.Minute)

	// No activity yet
	activity, err := s.GetTasksActivitySince(baseTime)
	if err != nil {
		t.Fatalf("Error getting activity: %v", err)
	}
	if activity.HasActivity() {
		t.Errorf("Expected no activity, got %+v", activity)
	}

	// Create a chore before baseTime
	oldChore := Chore{
		Name:    "Old Chore",
		Created: baseTime.Add(-1 * time.Hour),
	}
	_, err = s.SaveChore(oldChore)
	if err != nil {
		t.Fatalf("Error saving old chore: %v", err)
	}

	// Still no activity since baseTime
	activity, err = s.GetTasksActivitySince(baseTime)
	if err != nil {
		t.Fatalf("Error getting activity: %v", err)
	}
	if activity.HasActivity() {
		t.Errorf("Expected no activity since baseTime, got %+v", activity)
	}

	// Create a chore after baseTime
	newChore := Chore{
		Name:    "New Chore",
		Created: time.Now(),
	}
	savedNewChore, err := s.SaveChore(newChore)
	if err != nil {
		t.Fatalf("Error saving new chore: %v", err)
	}

	activity, err = s.GetTasksActivitySince(baseTime)
	if err != nil {
		t.Fatalf("Error getting activity: %v", err)
	}
	if !activity.HasActivity() {
		t.Error("Expected activity after creating new chore")
	}
	if len(activity.CreatedChores) != 1 {
		t.Errorf("Expected 1 created chore, got %d", len(activity.CreatedChores))
	}

	// Complete the new chore
	savedNewChore.Complete()
	_, err = s.SaveChore(savedNewChore)
	if err != nil {
		t.Fatalf("Error completing chore: %v", err)
	}

	activity, err = s.GetTasksActivitySince(baseTime)
	if err != nil {
		t.Fatalf("Error getting activity: %v", err)
	}
	if len(activity.CompletedChores) != 1 {
		t.Errorf("Expected 1 completed chore, got %d", len(activity.CompletedChores))
	}
}
