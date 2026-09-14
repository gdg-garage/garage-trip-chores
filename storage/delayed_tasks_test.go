package storage

import (
	"testing"
	"time"
)

func TestDelayedTasksStorage(t *testing.T) {
	s := createTestStorage(t)

	// Create a test chore
	chore, err := s.SaveChore(Chore{
		Name:             "Empty dishwasher",
		NecessaryWorkers: 1,
		EstimatedTimeMin: 10,
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	// Create delayed task scheduled for 10 minutes in the future
	publishAt := time.Now().Add(10 * time.Minute)
	dt, err := s.CreateDelayedTask(chore.ID, publishAt, 10)
	if err != nil {
		t.Fatalf("Failed to create delayed task: %v", err)
	}
	if dt.ID == 0 {
		t.Fatal("Expected non-zero ID for delayed task")
	}

	// Check pending tasks now: should be empty since publishAt is in future
	pending, err := s.GetPendingDelayedTasks(time.Now())
	if err != nil {
		t.Fatalf("Failed to get pending tasks: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Expected 0 pending tasks, got %d", len(pending))
	}

	// Check pending tasks with time in future: should return the task
	pending, err = s.GetPendingDelayedTasks(time.Now().Add(15 * time.Minute))
	if err != nil {
		t.Fatalf("Failed to get pending tasks: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending task, got %d", len(pending))
	}
	if pending[0].ChoreID != chore.ID {
		t.Fatalf("Expected ChoreID %d, got %d", chore.ID, pending[0].ChoreID)
	}

	// Get delayed task for chore
	found, err := s.GetDelayedTaskForChore(chore.ID)
	if err != nil {
		t.Fatalf("Failed to get delayed task for chore: %v", err)
	}
	if found == nil || found.ID != dt.ID {
		t.Fatalf("Expected delayed task with ID %d, got %v", dt.ID, found)
	}

	// Mark executed
	err = s.MarkDelayedTaskExecuted(dt.ID)
	if err != nil {
		t.Fatalf("Failed to mark delayed task executed: %v", err)
	}

	// Check pending tasks again with future time: should now be empty
	pending, err = s.GetPendingDelayedTasks(time.Now().Add(15 * time.Minute))
	if err != nil {
		t.Fatalf("Failed to get pending tasks: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Expected 0 pending tasks after execution, got %d", len(pending))
	}

	// Get delayed task for chore should now return nil
	found, err = s.GetDelayedTaskForChore(chore.ID)
	if err != nil {
		t.Fatalf("Failed to get delayed task for chore: %v", err)
	}
	if found != nil {
		t.Fatalf("Expected nil delayed task after execution, got %v", found)
	}
}

func TestCancelDelayedTaskByChoreId(t *testing.T) {
	s := createTestStorage(t)

	chore, err := s.SaveChore(Chore{
		Name: "Mow lawn",
	})
	if err != nil {
		t.Fatalf("Failed to save chore: %v", err)
	}

	publishAt := time.Now().Add(30 * time.Minute)
	_, err = s.CreateDelayedTask(chore.ID, publishAt, 30)
	if err != nil {
		t.Fatalf("Failed to create delayed task: %v", err)
	}

	// Cancel it
	err = s.CancelDelayedTaskByChoreId(chore.ID)
	if err != nil {
		t.Fatalf("Failed to cancel delayed task: %v", err)
	}

	// Check pending
	pending, err := s.GetPendingDelayedTasks(time.Now().Add(1 * time.Hour))
	if err != nil {
		t.Fatalf("Failed to get pending tasks: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("Expected 0 pending tasks after cancellation, got %d", len(pending))
	}
}
