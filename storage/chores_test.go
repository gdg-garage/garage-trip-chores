package storage

import (
	"testing"
	"time"
)

func TestDraftChores(t *testing.T) {
	s := createTestStorage(t)

	// Subscribing to event bus to verify drafts do not publish TaskCreated events
	ch := s.Events.Subscribe()
	defer s.Events.Unsubscribe(ch)

	// 1. Create a draft chore
	draftChore, err := s.SaveChore(Chore{
		Name:             "Draft chore",
		EstimatedTimeMin: 15,
		Draft:            true,
	})
	if err != nil {
		t.Fatalf("Failed to save draft chore: %v", err)
	}

	// Verify no event was published
	select {
	case event := <-ch:
		t.Fatalf("Unexpected event for draft chore: %+v", event)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}

	// 2. Create a non-draft chore
	normalChore, err := s.SaveChore(Chore{
		Name:             "Normal chore",
		EstimatedTimeMin: 20,
		Draft:            false,
	})
	if err != nil {
		t.Fatalf("Failed to save normal chore: %v", err)
	}

	// Verify event was published for normal chore
	select {
	case event := <-ch:
		if event.Type != TaskCreated || event.Chore.ID != normalChore.ID {
			t.Fatalf("Expected TaskCreated event for normal chore, got %+v", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected TaskCreated event for normal chore, timed out")
	}

	// 3. GetChores should only return the non-draft chore
	chores, err := s.GetChores()
	if err != nil {
		t.Fatalf("GetChores failed: %v", err)
	}
	if len(chores) != 1 {
		t.Fatalf("Expected 1 chore in GetChores, got %d", len(chores))
	}
	if chores[0].ID != normalChore.ID {
		t.Fatalf("Expected chore %d, got %d", normalChore.ID, chores[0].ID)
	}

	// 4. GetUnfinishedChores should only return the non-draft chore
	unfinished, err := s.GetUnfinishedChores()
	if err != nil {
		t.Fatalf("GetUnfinishedChores failed: %v", err)
	}
	if len(unfinished) != 1 || unfinished[0].ID != normalChore.ID {
		t.Fatalf("Expected 1 unfinished chore (%d), got %+v", normalChore.ID, unfinished)
	}

	// 5. GetChore should still be able to retrieve the draft chore by ID
	fetchedDraft, err := s.GetChore(draftChore.ID)
	if err != nil {
		t.Fatalf("GetChore failed for draft chore: %v", err)
	}
	if !fetchedDraft.Draft {
		t.Fatal("Expected fetchedDraft.Draft to be true")
	}

	// 6. DeleteChore should delete the draft chore
	err = s.DeleteChore(draftChore.ID)
	if err != nil {
		t.Fatalf("DeleteChore failed: %v", err)
	}
	_, err = s.GetChore(draftChore.ID)
	if err == nil {
		t.Fatal("Expected error when getting deleted draft chore, got nil")
	}
}

func TestLegacyDraftMigration(t *testing.T) {
	s := createTestStorage(t)

	// Simulate an orphaned preview chore that was created before the Draft column existed (draft = 0 / false)
	orphanedChore, err := s.SaveChore(Chore{
		Name:                 "Legacy orphaned preview",
		NecessaryWorkers:     1,
		EstimatedTimeMin:     10,
		AssignmentTimeoutMin: 15,
		CreatorId:            "12345",
		MessageId:            "",
		Draft:                false,
	})
	if err != nil {
		t.Fatalf("Failed to save orphaned chore: %v", err)
	}

	// Run the migration query manually
	res := s.db.Model(&Chore{}).Where("(draft = ? OR draft IS NULL) AND (message_id = '' OR message_id IS NULL) AND completed IS NULL AND (self_reported = ? OR self_reported IS NULL) AND id NOT IN (SELECT chore_id FROM delayed_tasks) AND id NOT IN (SELECT chore_id FROM chore_assignments)", false, false).Update("draft", true)
	if res.Error != nil {
		t.Fatalf("Migration query failed: %v", res.Error)
	}

	updated, err := s.GetChore(orphanedChore.ID)
	if err != nil {
		t.Fatalf("Failed to get chore after migration: %v", err)
	}
	if !updated.Draft {
		t.Fatal("Expected legacy orphaned chore to have Draft == true after migration")
	}

	// Should not be in GetChores
	chores, err := s.GetChores()
	if err != nil {
		t.Fatalf("GetChores failed: %v", err)
	}
	if len(chores) != 0 {
		t.Fatalf("Expected 0 chores in GetChores, got %d", len(chores))
	}
}
