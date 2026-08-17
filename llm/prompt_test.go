package llm

import (
	"strings"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func TestCalculateStatsDeltas(t *testing.T) {
	curr := map[string]storage.AggregatedUserStats{
		"u1": {WorkedMin: 50, WorkedCount: 2, PresentTicks: 10, NormalizedTotal: 5.0},
		"u2": {WorkedMin: 20, WorkedCount: 1, PresentTicks: 5, NormalizedTotal: 4.0},
	}
	prev := map[string]storage.AggregatedUserStats{
		"u1": {WorkedMin: 30, WorkedCount: 1, PresentTicks: 8, NormalizedTotal: 3.75},
		"u2": {WorkedMin: 20, WorkedCount: 1, PresentTicks: 5, NormalizedTotal: 4.0},
	}
	handles := map[string]string{
		"u1": "Alice",
		"u2": "Bob",
	}

	deltas := CalculateStatsDeltas(curr, prev, handles)
	if len(deltas) != 2 {
		t.Fatalf("Expected 2 deltas, got %d", len(deltas))
	}

	deltaMap := make(map[string]UserStatsDelta)
	for _, d := range deltas {
		deltaMap[d.UserId] = d
	}

	alice := deltaMap["u1"]
	if alice.Handle != "Alice" {
		t.Errorf("Expected handle Alice, got %s", alice.Handle)
	}
	if alice.DeltaWorkedMin != 20 {
		t.Errorf("Expected Alice DeltaWorkedMin 20, got %f", alice.DeltaWorkedMin)
	}
	if alice.DeltaWorkedCount != 1 {
		t.Errorf("Expected Alice DeltaWorkedCount 1, got %f", alice.DeltaWorkedCount)
	}

	bob := deltaMap["u2"]
	if bob.Handle != "Bob" {
		t.Errorf("Expected handle Bob, got %s", bob.Handle)
	}
	if bob.DeltaWorkedMin != 0 {
		t.Errorf("Expected Bob DeltaWorkedMin 0, got %f", bob.DeltaWorkedMin)
	}
}

func TestBuildPrompt(t *testing.T) {
	now := time.Now()
	activity := &storage.TasksActivity{
		CreatedChores: []storage.Chore{
			{ID: 1, Name: "Clean kitchen", CreatorId: "u1", EstimatedTimeMin: 15},
		},
		CompletedChores: []storage.Chore{
			{ID: 2, Name: "Take out trash", EstimatedTimeMin: 10},
		},
		WorkLogs: []storage.WorkLog{
			{ChoreId: 2, UserId: "u2", TimeSpentMin: 10},
		},
		UpdatedAssignments: []storage.ChoreAssignment{
			{UserId: "u3", Chore: storage.Chore{Name: "Fix grill"}, Timeouted: &now},
		},
	}

	curr := map[string]storage.AggregatedUserStats{
		"u1": {WorkedMin: 0},
		"u2": {WorkedMin: 10, WorkedCount: 1},
		"u3": {WorkedMin: 0},
	}
	prev := map[string]storage.AggregatedUserStats{}
	handles := map[string]string{
		"u1": "Alice",
		"u2": "Bob",
		"u3": "Charlie",
	}

	prompt, err := BuildPrompt(PromptData{
		StartTime:    now.Add(-6 * time.Hour),
		EndTime:      now,
		Activity:     activity,
		CurrentStats: curr,
		PrevStats:    prev,
		UserHandles:  handles,
	})
	if err != nil {
		t.Fatalf("BuildPrompt returned error: %v", err)
	}

	if !strings.Contains(prompt, "Clean kitchen") {
		t.Error("Prompt missing created chore 'Clean kitchen'")
	}
	if !strings.Contains(prompt, "Take out trash") {
		t.Error("Prompt missing completed chore 'Take out trash'")
	}
	if !strings.Contains(prompt, "Bob (10 min)") {
		t.Error("Prompt missing worker attribution for Bob")
	}
	if !strings.Contains(prompt, "Charlie") || !strings.Contains(prompt, "TIMED OUT") {
		t.Error("Prompt missing timed out assignment callout for Charlie")
	}
	if !strings.Contains(prompt, "Machine-Readable User Statistics") {
		t.Error("Prompt missing machine-readable statistics section")
	}
}
