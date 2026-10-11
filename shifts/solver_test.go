package shifts

import (
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func TestSolverNoOverlappingShifts(t *testing.T) {
	now := time.Now()
	users := []storage.User{
		{DiscordId: "u1", Name: "Alice", IsPresent: true},
		{DiscordId: "u2", Name: "Bob", IsPresent: true},
	}

	// Two overlapping shifts at the exact same time
	shifts := []storage.Shift{
		{
			ID:           1,
			Name:         "Kitchen Morning",
			StartTime:    now,
			EndTime:      now.Add(2 * time.Hour),
			WorkersCount: 1,
		},
		{
			ID:           2,
			Name:         "Bathroom Morning",
			StartTime:    now,
			EndTime:      now.Add(2 * time.Hour),
			WorkersCount: 1,
		},
	}

	input := SolverInput{
		Users:               users,
		Shifts:              shifts,
		NormalizedWorkloads: map[string]float64{"u1": 0, "u2": 0},
		PresentTicks:        map[string]int{"u1": 10, "u2": 10},
		ExistingDayCounts:   map[string]int{},
		MaxShiftsPerDay:     2,
	}

	solution := SolveOptimizedSchedule(input)
	if len(solution[1]) != 1 || len(solution[2]) != 1 {
		t.Fatalf("Expected 1 worker per shift, got %v and %v", solution[1], solution[2])
	}

	// Overlap constraint verification: u1 and u2 must not be assigned to the same user
	if solution[1][0] == solution[2][0] {
		t.Fatalf("Overlap violation! Same user %s assigned to overlapping shifts", solution[1][0])
	}
}

func TestSolverRewardsChoreMinutes(t *testing.T) {
	now := time.Now()
	// Alice did a heavy 120-minute chore. Bob did 0 minutes.
	users := []storage.User{
		{DiscordId: "u1", Name: "Alice", IsPresent: true},
		{DiscordId: "u2", Name: "Bob", IsPresent: true},
	}

	shifts := []storage.Shift{
		{
			ID:           1,
			Name:         "Kitchen Duty",
			StartTime:    now,
			EndTime:      now.Add(2 * time.Hour),
			WorkersCount: 1,
		},
	}

	input := SolverInput{
		Users:  users,
		Shifts: shifts,
		NormalizedWorkloads: map[string]float64{
			"u1": 12.0, // Alice has high prior workload
			"u2": 0.0,  // Bob has 0
		},
		PresentTicks:      map[string]int{"u1": 10, "u2": 10},
		ExistingDayCounts: map[string]int{},
		MaxShiftsPerDay:   1,
	}

	solution := SolveOptimizedSchedule(input)
	if len(solution[1]) == 0 {
		t.Fatalf("Expected shift to be assigned")
	}

	// Bob should be assigned because Alice worked chores
	assigned := solution[1][0]
	if assigned != "u2" {
		t.Fatalf("Expected Bob (u2) to be assigned to reward Alice for chores, but got %s", assigned)
	}
}

func TestSolverRequiresCapabilities(t *testing.T) {
	now := time.Now()
	users := []storage.User{
		{DiscordId: "u1", Name: "Novice", Capabilities: []string{}, IsPresent: true},
		{DiscordId: "u2", Name: "Chef", Capabilities: []string{"skill::cooking"}, IsPresent: true},
	}

	shift := storage.Shift{
		ID:           1,
		Name:         "Gourmet Dinner",
		StartTime:    now,
		EndTime:      now.Add(2 * time.Hour),
		WorkersCount: 1,
		Definition: storage.ShiftDefinition{
			NecessaryCapabilities: "skill::cooking",
		},
	}

	input := SolverInput{
		Users:               users,
		Shifts:              []storage.Shift{shift},
		NormalizedWorkloads: map[string]float64{"u1": 0, "u2": 10.0}, // Even though Chef has higher prior workload, Novice lacks the skill
		PresentTicks:        map[string]int{"u1": 10, "u2": 10},
		ExistingDayCounts:   map[string]int{},
		MaxShiftsPerDay:     1,
	}

	solution := SolveOptimizedSchedule(input)
	if len(solution[1]) == 0 {
		t.Fatalf("Expected shift to be assigned")
	}

	assigned := solution[1][0]
	if assigned != "u2" {
		t.Fatalf("Expected Chef (u2) to be assigned due to capability requirement, got %s", assigned)
	}
}

func TestSolverBuddyPairing(t *testing.T) {
	now := time.Now()
	users := []storage.User{
		{DiscordId: "u1", Name: "Novice", Capabilities: []string{}, IsPresent: true},
		{DiscordId: "u2", Name: "Chef", Capabilities: []string{"skill::kitchen"}, IsPresent: true},
		{DiscordId: "u3", Name: "OtherNovice", Capabilities: []string{}, IsPresent: true},
	}

	shift := storage.Shift{
		ID:           1,
		Name:         "Team Kitchen Shift",
		StartTime:    now,
		EndTime:      now.Add(2 * time.Hour),
		WorkersCount: 2, // Needs 2 workers, with skill::kitchen
		Definition: storage.ShiftDefinition{
			NecessaryCapabilities: "skill::kitchen",
		},
	}

	input := SolverInput{
		Users:               users,
		Shifts:              []storage.Shift{shift},
		NormalizedWorkloads: map[string]float64{"u1": 0, "u2": 0, "u3": 0},
		PresentTicks:        map[string]int{"u1": 10, "u2": 10, "u3": 10},
		ExistingDayCounts:   map[string]int{},
		MaxShiftsPerDay:     1,
	}

	solution := SolveOptimizedSchedule(input)
	assigned := solution[1]
	if len(assigned) != 2 {
		t.Fatalf("Expected 2 workers, got %v", assigned)
	}

	// One of them must be the chef
	hasChef := false
	for _, uid := range assigned {
		if uid == "u2" {
			hasChef = true
		}
	}
	if !hasChef {
		t.Fatalf("Expected buddy pairing to include Chef (u2) as lead, got %v", assigned)
	}
}
