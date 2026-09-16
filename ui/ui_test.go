package ui

import (
	"fmt"
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

func TestCompleteChore_CreditsClicker(t *testing.T) {
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

	// Chore created without any acked workers
	chore, err := s.SaveChore(storage.Chore{
		Name:             "Clean floor",
		CreatorId:        "creator1",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 25,
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	// User 'volunteer_clicker' marks it done
	completed, err := ui.CompleteChore(chore.ID, "volunteer_clicker")
	if err != nil {
		t.Fatalf("CompleteChore failed: %v", err)
	}
	if completed.Completed == nil {
		t.Fatal("Expected chore to be completed")
	}

	// Verify worklog was created for volunteer_clicker
	wls, err := s.GetWorkLogsForChore(chore.ID)
	if err != nil {
		t.Fatalf("Failed to get worklogs: %v", err)
	}
	if len(wls) != 1 {
		t.Fatalf("Expected 1 worklog credited to clicker, got %d", len(wls))
	}
	if wls[0].UserId != "volunteer_clicker" || wls[0].TimeSpentMin != 25 {
		t.Fatalf("Unexpected worklog: %+v", wls[0])
	}
}

func TestAckAndReject_StateGuards(t *testing.T) {
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
	completedChore, _ := s.SaveChore(storage.Chore{
		Name:             "Done chore",
		CreatorId:        "c1",
		EstimatedTimeMin: 10,
		Completed:        &now,
	})

	cancelledChore, _ := s.SaveChore(storage.Chore{
		Name:             "Cancelled chore",
		CreatorId:        "c1",
		EstimatedTimeMin: 10,
		Cancelled:        &now,
	})

	// Try Acking completed chore
	_, _, err = ui.AckChore(completedChore.ID, "u1")
	if err == nil || !strings.Contains(err.Error(), "already been completed") {
		t.Fatalf("Expected 'already been completed' error, got %v", err)
	}

	// Try Rejecting completed chore
	_, err = ui.RejectChore(completedChore.ID, "u1")
	if err == nil || !strings.Contains(err.Error(), "already been completed") {
		t.Fatalf("Expected 'already been completed' error, got %v", err)
	}

	// Try Acking cancelled chore
	_, _, err = ui.AckChore(cancelledChore.ID, "u1")
	if err == nil || !strings.Contains(err.Error(), "has been cancelled") {
		t.Fatalf("Expected 'has been cancelled' error, got %v", err)
	}

	// Try Rejecting cancelled chore
	_, err = ui.RejectChore(cancelledChore.ID, "u1")
	if err == nil || !strings.Contains(err.Error(), "has been cancelled") {
		t.Fatalf("Expected 'has been cancelled' error, got %v", err)
	}
}

func TestGetOverachieverNudge(t *testing.T) {
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

	// Log work for 4 users: 1 overachiever (100 mins) and 3 normal performers (10 mins each)
	// Total = 130 mins, Average = 32.5 mins, 2x Average = 65 mins.
	// Overachiever has 100 mins >= 65 mins -> gets nudge.
	chore1, _ := s.SaveChore(storage.Chore{Name: "c1", EstimatedTimeMin: 100})
	chore2, _ := s.SaveChore(storage.Chore{Name: "c2", EstimatedTimeMin: 10})

	_, _ = s.SaveWorkLog(storage.WorkLog{ChoreId: chore1.ID, UserId: "overachiever", TimeSpentMin: 100})
	_, _ = s.SaveWorkLog(storage.WorkLog{ChoreId: chore2.ID, UserId: "user_b", TimeSpentMin: 10})
	_, _ = s.SaveWorkLog(storage.WorkLog{ChoreId: chore2.ID, UserId: "user_c", TimeSpentMin: 10})
	_, _ = s.SaveWorkLog(storage.WorkLog{ChoreId: chore2.ID, UserId: "user_d", TimeSpentMin: 10})

	nudgeHigh := ui.getOverachieverNudge("overachiever")
	if nudgeHigh == "" || !strings.Contains(nudgeHigh, "Friendly nudge") {
		t.Fatalf("Expected overachiever nudge, got %q", nudgeHigh)
	}

	nudgeRegular := ui.getOverachieverNudge("user_b")
	if nudgeRegular != "" {
		t.Fatalf("Expected no nudge for regular worker, got %q", nudgeRegular)
	}
}

func TestBuildChoreComponents(t *testing.T) {
	now := time.Now()

	// 1. Open chore with no acks -> [Ack, Reject]
	choreUnacked := storage.Chore{ID: 10}
	comps := buildChoreComponents(choreUnacked, 0)
	if len(comps) != 1 {
		t.Fatalf("Expected 1 ActionsRow, got %d", len(comps))
	}
	row, ok := comps[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("Expected ActionsRow type")
	}
	if len(row.Components) != 2 {
		t.Fatalf("Expected 2 buttons for unacked chore, got %d", len(row.Components))
	}
	btn0 := row.Components[0].(*discordgo.Button)
	btn1 := row.Components[1].(*discordgo.Button)
	if btn0.CustomID != AckButtonClick+"10" || btn1.CustomID != RejectButtonClick+"10" {
		t.Fatalf("Unexpected buttons: %+v, %+v", btn0, btn1)
	}

	// 2. Open chore with acks -> [Ack, Reject, Done!]
	compsAcked := buildChoreComponents(choreUnacked, 1)
	if len(compsAcked) != 1 {
		t.Fatalf("Expected 1 ActionsRow, got %d", len(compsAcked))
	}
	rowAcked := compsAcked[0].(discordgo.ActionsRow)
	if len(rowAcked.Components) != 3 {
		t.Fatalf("Expected 3 buttons for acked chore, got %d", len(rowAcked.Components))
	}
	btnDone := rowAcked.Components[2].(*discordgo.Button)
	if btnDone.CustomID != DoneButtonClick+"10" || btnDone.Label != "Done!" {
		t.Fatalf("Unexpected Done button: %+v", btnDone)
	}

	// 3. Completed chore -> [I helped]
	choreCompleted := storage.Chore{ID: 20, Completed: &now}
	compsCompleted := buildChoreComponents(choreCompleted, 1)
	if len(compsCompleted) != 1 {
		t.Fatalf("Expected 1 ActionsRow, got %d", len(compsCompleted))
	}
	rowCompleted := compsCompleted[0].(discordgo.ActionsRow)
	if len(rowCompleted.Components) != 1 {
		t.Fatalf("Expected 1 button for completed chore, got %d", len(rowCompleted.Components))
	}
	btnHelped := rowCompleted.Components[0].(*discordgo.Button)
	if btnHelped.CustomID != HelpedButtonClick+"20" || btnHelped.Label != "I helped" {
		t.Fatalf("Unexpected Helped button: %+v", btnHelped)
	}

	// 4. Cancelled chore -> no buttons
	choreCancelled := storage.Chore{ID: 30, Cancelled: &now}
	compsCancelled := buildChoreComponents(choreCancelled, 0)
	if len(compsCancelled) != 0 {
		t.Fatalf("Expected 0 components for cancelled chore, got %d", len(compsCancelled))
	}
}

func TestBuildChoreMessageContent(t *testing.T) {
	now := time.Now()

	// 1. Completed
	cComp := storage.Chore{Name: "Wash dishes", Completed: &now}
	if got := buildChoreMessageContent(cComp, nil, nil); got != "✅ Wash dishes" {
		t.Fatalf("Expected completed prefix, got %q", got)
	}

	// 2. Cancelled
	cCanc := storage.Chore{Name: "Wash dishes", Cancelled: &now}
	if got := buildChoreMessageContent(cCanc, nil, nil); got != "❌ Wash dishes" {
		t.Fatalf("Expected cancelled prefix, got %q", got)
	}

	// 3. Direct Assignee
	cDirect := storage.Chore{Name: "Fix door", AssigneeId: "u123"}
	if got := buildChoreMessageContent(cDirect, nil, nil); got != "Fix door — Assigned to <@u123>" {
		t.Fatalf("Expected direct assignee mention, got %q", got)
	}

	// 4. Multiple active assignments
	cAssigned := storage.Chore{Name: "Cook dinner"}
	assignments := []storage.ChoreAssignment{
		{UserId: "u1"},
		{UserId: "u2"},
	}
	if got := buildChoreMessageContent(cAssigned, assignments, nil); got != "Cook dinner — Assigned to <@u1>, <@u2>" {
		t.Fatalf("Expected multiple assignee mentions, got %q", got)
	}

	// 5. Claimed/acked
	acked := []storage.ChoreAssignment{
		{UserId: "u1"},
	}
	if got := buildChoreMessageContent(cAssigned, nil, acked); got != "Cook dinner — Claimed by <@u1>" {
		t.Fatalf("Expected claimed by mention, got %q", got)
	}

	// 6. Plain chore
	if got := buildChoreMessageContent(cAssigned, nil, nil); got != "Cook dinner" {
		t.Fatalf("Expected plain chore name, got %q", got)
	}
}

func TestBuildChoresListResponse(t *testing.T) {
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

	// 1. User with no chores
	respEmpty, err := ui.buildChoresListResponse("user_empty")
	if err != nil {
		t.Fatalf("Failed to build chores list for empty user: %v", err)
	}
	if len(respEmpty.Data.Embeds) != 1 || respEmpty.Data.Embeds[0].Title != "No chores found!" {
		t.Fatalf("Expected 'No chores found!' embed, got %+v", respEmpty.Data.Embeds)
	}
	if len(respEmpty.Data.Components) != 0 {
		t.Fatalf("Expected no components for empty chores list, got %d", len(respEmpty.Data.Components))
	}

	// 2. User with 1 assigned chore and 1 acked chore
	choreAssigned, _ := s.SaveChore(storage.Chore{Name: "Clean garage", CreatorId: "c1", EstimatedTimeMin: 15})
	_, _ = s.SaveChoreAssignment(storage.ChoreAssignment{
		ChoreId: choreAssigned.ID,
		UserId:  "user_active",
		Created: time.Now(),
	})

	choreAcked, _ := s.SaveChore(storage.Chore{Name: "Cook dinner", CreatorId: "c1", EstimatedTimeMin: 30})
	assAcked, _ := s.SaveChoreAssignment(storage.ChoreAssignment{
		ChoreId: choreAcked.ID,
		UserId:  "user_active",
		Created: time.Now(),
	})
	assAcked.Ack()
	_, _ = s.SaveChoreAssignment(assAcked)

	resp, err := ui.buildChoresListResponse("user_active")
	if err != nil {
		t.Fatalf("Failed to build chores list for active user: %v", err)
	}
	if len(resp.Data.Embeds) != 2 {
		t.Fatalf("Expected 2 embeds (assigned + acked), got %d", len(resp.Data.Embeds))
	}

	// Components should contain:
	// Row 0: Done! button for Cook dinner
	// Row 1: Ack & Reject buttons for Clean garage
	if len(resp.Data.Components) != 2 {
		t.Fatalf("Expected 2 component rows, got %d", len(resp.Data.Components))
	}

	rowDone := resp.Data.Components[0].(discordgo.ActionsRow)
	if len(rowDone.Components) != 1 {
		t.Fatalf("Expected 1 Done button, got %d", len(rowDone.Components))
	}
	doneBtn := rowDone.Components[0].(*discordgo.Button)
	if doneBtn.CustomID != DoneButtonClick+fmt.Sprint(choreAcked.ID) || !strings.Contains(doneBtn.Label, "Done:") {
		t.Fatalf("Unexpected Done button: %+v", doneBtn)
	}

	rowAssigned := resp.Data.Components[1].(discordgo.ActionsRow)
	if len(rowAssigned.Components) != 2 {
		t.Fatalf("Expected 2 buttons for assigned chore, got %d", len(rowAssigned.Components))
	}
	ackBtn := rowAssigned.Components[0].(*discordgo.Button)
	rejectBtn := rowAssigned.Components[1].(*discordgo.Button)
	if ackBtn.CustomID != AckButtonClick+fmt.Sprint(choreAssigned.ID) || !strings.Contains(ackBtn.Label, "Ack:") {
		t.Fatalf("Unexpected Ack button: %+v", ackBtn)
	}
	if rejectBtn.CustomID != RejectButtonClick+fmt.Sprint(choreAssigned.ID) || !strings.Contains(rejectBtn.Label, "Reject:") {
		t.Fatalf("Unexpected Reject button: %+v", rejectBtn)
	}
}



