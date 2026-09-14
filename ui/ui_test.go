package ui

import (
	"log/slog"
	"os"
	"path/filepath"
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

