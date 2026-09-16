package ui

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

func TestGetChoreIdFromButton(t *testing.T) {
	tests := []struct {
		name      string
		customID  string
		wantID    uint
		expectErr bool
	}{
		{
			name:      "valid custom ID",
			customID:  "schedule_button_click:123",
			wantID:    123,
			expectErr: false,
		},
		{
			name:      "valid custom ID with zero",
			customID:  "delete_button_click:0",
			wantID:    0,
			expectErr: false,
		},
		{
			name:      "invalid format - no colon",
			customID:  "invalid_id",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "invalid format - too many parts",
			customID:  "button:123:extra",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "invalid chore ID - not a number",
			customID:  "button:abc",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "invalid chore ID - empty",
			customID:  "button:",
			wantID:    0,
			expectErr: true,
		},
		{
			name:      "empty custom ID",
			customID:  "",
			wantID:    0,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := getChoreIdFromCustomID(tt.customID)

			if (err != nil) != tt.expectErr {
				t.Errorf("getChoreIdFromButton() error = %v, expectErr %v", err, tt.expectErr)
				return
			}

			if gotID != tt.wantID {
				t.Errorf("getChoreIdFromButton() gotID = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestGetInteractionUserId(t *testing.T) {
	tests := []struct {
		name     string
		input    *discordgo.InteractionCreate
		expected string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
		{
			name:     "empty interaction",
			input:    &discordgo.InteractionCreate{},
			expected: "",
		},
		{
			name: "from interaction user",
			input: &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					User: &discordgo.User{ID: "user_123"},
				},
			},
			expected: "user_123",
		},
		{
			name: "from interaction member user",
			input: &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					Member: &discordgo.Member{
						User: &discordgo.User{ID: "member_456"},
					},
				},
			},
			expected: "member_456",
		},
		{
			name: "member without user",
			input: &discordgo.InteractionCreate{
				Interaction: &discordgo.Interaction{
					Member: &discordgo.Member{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getInteractionUserId(tt.input)
			if got != tt.expected {
				t.Errorf("getInteractionUserId() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestProcessPendingDelayedTasks(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := storage.New(storage.Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	cl := chores.NewChoresLogic(s, logger, chores.Config{})
	ui := NewUi(s, logger, &cl, nil, Config{})

	// Create a chore with delay
	chore, err := s.SaveChore(storage.Chore{
		Name:             "Delayed clean",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 15,
		DelayMin:         10,
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	// Create delayed task scheduled in the past
	publishAt := time.Now().Add(-1 * time.Minute)
	_, err = s.CreateDelayedTask(chore.ID, publishAt, 10)
	if err != nil {
		t.Fatalf("Failed to create delayed task: %v", err)
	}

	// Process pending delayed tasks
	ui.ProcessPendingDelayedTasks()

	// Verify delayed task is marked executed
	pending, err := s.GetPendingDelayedTasks(time.Now())
	if err != nil {
		t.Fatalf("Failed to get pending tasks: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Expected 0 pending tasks after processing, got %d", len(pending))
	}

	// Verify chore has been published
	updatedChore, err := s.GetChore(chore.ID)
	if err != nil {
		t.Fatalf("Failed to get chore: %v", err)
	}
	if updatedChore.ID != chore.ID {
		t.Fatalf("Expected chore ID %d, got %d", chore.ID, updatedChore.ID)
	}

	// Test case: Cancelled chore should be marked executed without error
	chore2, err := s.SaveChore(storage.Chore{
		Name:             "Cancelled chore",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 10,
		DelayMin:         5,
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}
	cancelledTime := time.Now()
	chore2.Cancelled = &cancelledTime
	chore2, _ = s.SaveChore(chore2)

	dt2, err := s.CreateDelayedTask(chore2.ID, time.Now().Add(-1*time.Minute), 5)
	if err != nil {
		t.Fatalf("Failed to create delayed task: %v", err)
	}

	ui.ProcessPendingDelayedTasks()

	// Should have marked dt2 executed
	dt2Found, err := s.GetDelayedTaskForChore(dt2.ChoreID)
	if err != nil {
		t.Fatalf("Error getting delayed task: %v", err)
	}
	if dt2Found != nil {
		t.Fatalf("Expected dt2 to be marked executed, but still found")
	}
}

func TestCancelChore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := storage.New(storage.Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	cl := chores.NewChoresLogic(s, logger, chores.Config{})
	ui := NewUi(s, logger, &cl, nil, Config{})

	// Create a chore
	chore, err := s.SaveChore(storage.Chore{
		Name:             "Chore to cancel",
		CreatorId:        "user_creator",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 15,
		DelayMin:         10,
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	// Create an assignment
	_, err = s.SaveChoreAssignment(storage.ChoreAssignment{
		ChoreId: chore.ID,
		UserId:  "worker_1",
	})
	if err != nil {
		t.Fatalf("Failed to create chore assignment: %v", err)
	}

	// Create a delayed task
	_, err = s.CreateDelayedTask(chore.ID, time.Now().Add(10*time.Minute), 10)
	if err != nil {
		t.Fatalf("Failed to create delayed task: %v", err)
	}

	// Cancel the chore
	cancelled, err := ui.CancelChore(chore.ID)
	if err != nil {
		t.Fatalf("CancelChore failed: %v", err)
	}
	if cancelled.Cancelled == nil {
		t.Fatal("Expected chore.Cancelled to be non-nil")
	}

	// Verify assignments removed
	assignments, err := s.GetChoreAssignments(chore.ID)
	if err != nil {
		t.Fatalf("Failed to get assignments: %v", err)
	}
	if len(assignments) != 0 {
		t.Fatalf("Expected 0 assignments after cancellation, got %d", len(assignments))
	}

	// Verify delayed task cancelled
	dt, err := s.GetDelayedTaskForChore(chore.ID)
	if err != nil {
		t.Fatalf("Failed to get delayed task: %v", err)
	}
	if dt != nil {
		t.Fatal("Expected delayed task to be removed after cancellation")
	}

	// Cancelling again should return error
	_, err = ui.CancelChore(chore.ID)
	if err == nil {
		t.Fatal("Expected error when cancelling already cancelled chore, got nil")
	}
}

func TestCancelChore_Completed(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := storage.New(storage.Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	cl := chores.NewChoresLogic(s, logger, chores.Config{})
	ui := NewUi(s, logger, &cl, nil, Config{})

	now := time.Now()
	chore, err := s.SaveChore(storage.Chore{
		Name:             "Completed chore",
		CreatorId:        "user_creator",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 15,
		Completed:        &now,
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	_, err = ui.CancelChore(chore.ID)
	if err == nil {
		t.Fatal("Expected error when cancelling completed chore, got nil")
	}
}

func TestPublishSelfReportedChore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := storage.New(storage.Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	cl := chores.NewChoresLogic(s, logger, chores.Config{})
	ui := NewUi(s, logger, &cl, nil, Config{})

	chore := storage.Chore{
		Name:             "Watered plants",
		CreatorId:        "plant_lover",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 15,
		SelfReported:     true,
	}

	published, assignments, err := ui.PublishChore(chore)
	if err != nil {
		t.Fatalf("PublishChore failed: %v", err)
	}

	if !published.SelfReported {
		t.Fatal("Expected published chore SelfReported to be true")
	}
	if published.Completed == nil {
		t.Fatal("Expected published chore Completed to be non-nil")
	}
	if len(assignments) != 1 {
		t.Fatalf("Expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].UserId != "plant_lover" || assignments[0].Acked == nil || !assignments[0].Volunteered {
		t.Fatalf("Expected acked volunteered assignment for plant_lover: %+v", assignments[0])
	}

	// Verify storage state
	storedChore, err := s.GetChore(published.ID)
	if err != nil {
		t.Fatalf("Failed to get chore: %v", err)
	}
	if !storedChore.SelfReported || storedChore.Completed == nil {
		t.Fatalf("Stored chore not marked as self-reported completed: %+v", storedChore)
	}

	storedAssignments, err := s.GetChoreAssignments(published.ID)
	if err != nil {
		t.Fatalf("Failed to get assignments: %v", err)
	}
	if len(storedAssignments) != 1 || storedAssignments[0].UserId != "plant_lover" || storedAssignments[0].Acked == nil {
		t.Fatalf("Expected 1 acked assignment in storage: %+v", storedAssignments)
	}

	worklogs, err := s.GetWorkLogsForChore(published.ID)
	if err != nil {
		t.Fatalf("Failed to get worklogs: %v", err)
	}
	if len(worklogs) != 1 {
		t.Fatalf("Expected 1 worklog, got %d", len(worklogs))
	}
	if worklogs[0].UserId != "plant_lover" || worklogs[0].TimeSpentMin != 15 || !worklogs[0].SelfReported {
		t.Fatalf("Unexpected worklog in storage: %+v", worklogs[0])
	}
}

func TestPublishChoreWithDirectAssignee(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	s, err := storage.New(storage.Config{
		DbPath: dbPath,
	}, logger)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	cl := chores.NewChoresLogic(s, logger, chores.Config{})
	ui := NewUi(s, logger, &cl, nil, Config{})

	chore := storage.Chore{
		Name:             "Direct assigned chore",
		CreatorId:        "creator_user",
		AssigneeId:       "direct_worker_123",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 20,
	}

	published, assignments, err := ui.PublishChore(chore)
	if err != nil {
		t.Fatalf("PublishChore failed: %v", err)
	}

	if published.AssigneeId != "direct_worker_123" {
		t.Fatalf("Expected published chore AssigneeId to be 'direct_worker_123', got %s", published.AssigneeId)
	}

	if len(assignments) != 1 {
		t.Fatalf("Expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].UserId != "direct_worker_123" {
		t.Fatalf("Expected assignment for direct_worker_123, got %s", assignments[0].UserId)
	}

	// Verify storage state
	storedAssignments, err := s.GetChoreAssignments(published.ID)
	if err != nil {
		t.Fatalf("Failed to get assignments: %v", err)
	}
	if len(storedAssignments) != 1 || storedAssignments[0].UserId != "direct_worker_123" {
		t.Fatalf("Expected 1 assignment in storage for direct_worker_123: %+v", storedAssignments)
	}
}

func TestUserMentionExtraction(t *testing.T) {
	tests := []struct {
		inputName      string
		expectedName   string
		expectedUserId string
	}{
		{
			inputName:      "Wash dishes <@123456789>",
			expectedName:   "Wash dishes",
			expectedUserId: "123456789",
		},
		{
			inputName:      "<@!987654321> Cook dinner",
			expectedName:   "Cook dinner",
			expectedUserId: "987654321",
		},
		{
			inputName:      "Take out trash",
			expectedName:   "Take out trash",
			expectedUserId: "",
		},
		{
			inputName:      "Clean table <@111222> after lunch",
			expectedName:   "Clean table  after lunch",
			expectedUserId: "111222",
		},
	}

	for _, tt := range tests {
		var extractedUserId string
		name := tt.inputName
		if matches := userMentionRegex.FindStringSubmatch(name); len(matches) > 1 {
			extractedUserId = matches[1]
			cleanName := strings.TrimSpace(userMentionRegex.ReplaceAllString(name, ""))
			if cleanName != "" {
				name = cleanName
			}
		}

		if extractedUserId != tt.expectedUserId {
			t.Errorf("For %q, expected user ID %q, got %q", tt.inputName, tt.expectedUserId, extractedUserId)
		}
		if name != tt.expectedName {
			t.Errorf("For %q, expected clean name %q, got %q", tt.inputName, tt.expectedName, name)
		}
	}
}



