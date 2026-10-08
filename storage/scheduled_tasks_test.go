package storage

import (
	"testing"
	"time"
)

func TestScheduledTasksStorage(t *testing.T) {
	s := createTestStorage(t)

	now := time.Now()
	next := now.Add(2 * time.Hour)

	task, err := s.CreateScheduledTask(ScheduledTask{
		Name:                  "Take out garbage",
		CronExpr:              "0 9 * * *",
		NecessaryWorkers:      1,
		EstimatedTimeMin:      10,
		AssignmentTimeoutMin:  15,
		NecessaryCapabilities: "[]",
		CreatorId:             "user-123",
		CreatorName:           "Dominik",
		Enabled:               true,
		NextRunAt:             &next,
	})
	if err != nil {
		t.Fatalf("Failed to create scheduled task: %v", err)
	}
	if task.ID == 0 {
		t.Fatal("Expected non-zero task ID")
	}

	// Fetch by ID
	found, err := s.GetScheduledTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to get scheduled task: %v", err)
	}
	if found == nil || found.Name != "Take out garbage" || found.CreatorId != "user-123" {
		t.Fatalf("Unexpected task fetched: %+v", found)
	}

	// Check due tasks (should be 0 right now)
	due, err := s.GetDueScheduledTasks(now)
	if err != nil {
		t.Fatalf("Failed to get due tasks: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("Expected 0 due tasks, got %d", len(due))
	}

	// Check due tasks in future (after 2 hours)
	due, err = s.GetDueScheduledTasks(now.Add(3 * time.Hour))
	if err != nil {
		t.Fatalf("Failed to get due tasks: %v", err)
	}
	if len(due) != 1 || due[0].ID != task.ID {
		t.Fatalf("Expected 1 due task with ID %d, got %+v", task.ID, due)
	}

	// Update task
	task.Name = "Take out all trash"
	task.Enabled = false
	err = s.UpdateScheduledTask(task)
	if err != nil {
		t.Fatalf("Failed to update task: %v", err)
	}

	found, _ = s.GetScheduledTask(task.ID)
	if found.Name != "Take out all trash" || found.Enabled != false {
		t.Fatalf("Expected updated name and disabled status, got %+v", found)
	}

	// When disabled, shouldn't appear in due tasks
	due, _ = s.GetDueScheduledTasks(now.Add(3 * time.Hour))
	if len(due) != 0 {
		t.Fatalf("Expected 0 due tasks when disabled, got %d", len(due))
	}

	// Update run timestamps
	lastRun := time.Now()
	nextRun := lastRun.Add(24 * time.Hour)
	err = s.UpdateScheduledTaskRun(task.ID, lastRun, &nextRun)
	if err != nil {
		t.Fatalf("Failed to update run timestamps: %v", err)
	}
	found, _ = s.GetScheduledTask(task.ID)
	if found.LastRunAt == nil || found.NextRunAt == nil {
		t.Fatalf("Expected non-nil LastRunAt and NextRunAt, got %+v", found)
	}

	// Delete
	err = s.DeleteScheduledTask(task.ID)
	if err != nil {
		t.Fatalf("Failed to delete task: %v", err)
	}
	found, err = s.GetScheduledTask(task.ID)
	if err != nil || found != nil {
		t.Fatalf("Expected task to be nil after deletion, got err=%v, found=%v", err, found)
	}
}
