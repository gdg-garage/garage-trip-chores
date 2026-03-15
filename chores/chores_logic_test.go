package chores

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type MockStorage struct {
	Stats       storage.UserChoreStats
	Assignments []storage.ChoreAssignment
}

func (m *MockStorage) GetTotalNormalizedChoreStats() (storage.UserChoreStats, error) {
	return m.Stats, nil
}

func (m *MockStorage) GetChoreAssignments(choreId uint) ([]storage.ChoreAssignment, error) {
	var ass []storage.ChoreAssignment
	for _, a := range m.Assignments {
		if a.ChoreId == choreId {
			ass = append(ass, a)
		}
	}
	return ass, nil
}

func (m *MockStorage) SaveChoreAssignments(assignments []storage.ChoreAssignment) ([]storage.ChoreAssignment, error) {
	m.Assignments = append(m.Assignments, assignments...)
	return assignments, nil
}

func TestAssignChoresToUsers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockStorage := &MockStorage{
		Stats: storage.UserChoreStats{
			"u1": {Count: 0, TotalMin: 0},
			"u2": {Count: 1, TotalMin: 10},
			"u3": {Count: 2, TotalMin: 20},
		},
		Assignments: []storage.ChoreAssignment{},
	}

	cl := NewChoresLogic(mockStorage, logger, Config{OversampleRatio: 0.0})

	users := []storage.User{
		{DiscordId: "u1", Capabilities: []string{"drv"}},
		{DiscordId: "u2", Capabilities: []string{"drv", "cook"}},
		{DiscordId: "u3", Capabilities: []string{"cook"}},
	}

	chore := storage.Chore{
		ID:                    1,
		NecessaryWorkers:      2,
		NecessaryCapabilities: "drv",
	}

	assignments, err := cl.AssignChoresToUsers(users, chore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(assignments))
	}

	// Because of our sorting logic:
	// Capabilities matched max -> min (descending)
	// u1 matches 1, u2 matches 1, u3 matches 0
	// For capabilities matches equal (u1 vs u2):
	// TotalMin min -> max (ascending)
	// u1 TotalMin=0, u2 TotalMin=10
	// => top 2 are u1 and u2.

	foundU1 := false
	foundU2 := false
	for _, a := range assignments {
		if a.UserId == "u1" {
			foundU1 = true
		}
		if a.UserId == "u2" {
			foundU2 = true
		}
	}

	if !foundU1 || !foundU2 {
		t.Fatalf("expected u1 and u2 to be assigned, assignments were: %v", assignments)
	}

	// Test case 2: u1 and u2 are active assigned, if we need 3 workers, u3 gets assigned
	chore.NecessaryWorkers = 3
	assignments2, err := cl.AssignChoresToUsers(users, chore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	if len(assignments2) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments2))
	}
	if assignments2[0].UserId != "u3" {
		t.Fatalf("expected u3, got %s", assignments2[0].UserId)
	}

	// Test case 3: Refused assignments don't count towards alreadyAssignedCnt, but they still eliminate the user from pool.
	now := time.Now()
	mockStorage.Assignments[2].Refused = &now
	chore.NecessaryWorkers = 3
	assignments3, err := cl.AssignChoresToUsers(users, chore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments3) != 0 {
		t.Fatalf("expected 0 assignment, got %d", len(assignments3))
	}
	
	// Test case 4: If timeouted, also doesn't assign again
	mockStorage.Assignments[2].Refused = nil
	mockStorage.Assignments[2].Timeouted = &now
	assignments4, err := cl.AssignChoresToUsers(users, chore)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(assignments4) != 0 {
		t.Fatalf("expected 0 assignment, got %d", len(assignments4))
	}
}
