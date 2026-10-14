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

func TestIsUserEligibleForDate(t *testing.T) {
	today := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	tomorrow := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	dayAfter := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	nextWeek := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

	uAlice := storage.User{DiscordId: "alice", IsPresent: false}
	uBob := storage.User{DiscordId: "bob", IsPresent: true}
	uCharlie := storage.User{DiscordId: "charlie", IsPresent: false}

	regs := map[string]storage.UserRegistration{
		// Alice arrives tomorrow (Oct 16) and stays until Oct 18
		"alice": {
			DiscordId:     "alice",
			ArrivalDate:   time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC),
			DepartureDate: time.Date(2026, 10, 18, 18, 0, 0, 0, time.UTC),
			Cancelled:     false,
		},
		// Bob is currently present today (Oct 15) but departs today evening
		"bob": {
			DiscordId:     "bob",
			ArrivalDate:   time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC),
			DepartureDate: time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC),
			Cancelled:     false,
		},
		// Charlie registered for tomorrow but cancelled
		"charlie": {
			DiscordId:     "charlie",
			ArrivalDate:   time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC),
			DepartureDate: time.Date(2026, 10, 18, 18, 0, 0, 0, time.UTC),
			Cancelled:     true,
		},
	}

	// 1. Alice is arriving tomorrow: not eligible today, eligible tomorrow and dayAfter, not eligible nextWeek
	if IsUserEligibleForDate(uAlice, today, regs) {
		t.Errorf("Alice should not be eligible today (before arrival)")
	}
	if !IsUserEligibleForDate(uAlice, tomorrow, regs) {
		t.Errorf("Alice should be eligible tomorrow (arrival date)")
	}
	if !IsUserEligibleForDate(uAlice, dayAfter, regs) {
		t.Errorf("Alice should be eligible day after tomorrow")
	}
	if IsUserEligibleForDate(uAlice, nextWeek, regs) {
		t.Errorf("Alice should not be eligible next week (after departure)")
	}

	// 2. Bob departs today: eligible today, NOT eligible tomorrow
	if !IsUserEligibleForDate(uBob, today, regs) {
		t.Errorf("Bob should be eligible today (prior to departure)")
	}
	if IsUserEligibleForDate(uBob, tomorrow, regs) {
		t.Errorf("Bob should NOT be eligible tomorrow (departs today, prevents swap disaster)")
	}

	// 3. Charlie cancelled: not eligible
	if IsUserEligibleForDate(uCharlie, tomorrow, regs) {
		t.Errorf("Charlie should NOT be eligible because registration is cancelled")
	}

	// 4. Dave has no registration record: fallback to IsPresent
	uDavePresent := storage.User{DiscordId: "dave", IsPresent: true}
	uDaveAbsent := storage.User{DiscordId: "dave", IsPresent: false}
	if !IsUserEligibleForDate(uDavePresent, tomorrow, regs) {
		t.Errorf("Dave should be eligible via fallback IsPresent=true")
	}
	if IsUserEligibleForDate(uDaveAbsent, tomorrow, regs) {
		t.Errorf("Dave should not be eligible via fallback IsPresent=false")
	}
}

func TestSolverPredictsArrivalAndDepartureWithRegistrations(t *testing.T) {
	tomorrow := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)

	// Alice is NOT currently present today, but arrives tomorrow
	// Bob IS currently present today, but leaves today
	users := []storage.User{
		{DiscordId: "alice", Name: "Alice", IsPresent: false},
		{DiscordId: "bob", Name: "Bob", IsPresent: true},
	}

	regs := map[string]storage.UserRegistration{
		"alice": {
			DiscordId:     "alice",
			ArrivalDate:   time.Date(2026, 10, 16, 8, 0, 0, 0, time.UTC),
			DepartureDate: time.Date(2026, 10, 18, 18, 0, 0, 0, time.UTC),
			Cancelled:     false,
		},
		"bob": {
			DiscordId:     "bob",
			ArrivalDate:   time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC),
			DepartureDate: time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC),
			Cancelled:     false,
		},
	}

	shifts := []storage.Shift{
		{
			ID:           1,
			Name:         "Kitchen Duty Tomorrow",
			StartTime:    tomorrow,
			EndTime:      tomorrow.Add(2 * time.Hour),
			WorkersCount: 1,
		},
	}

	input := SolverInput{
		Users:               users,
		Shifts:              shifts,
		NormalizedWorkloads: map[string]float64{"alice": 0, "bob": 0},
		PresentTicks:        map[string]int{"alice": 1, "bob": 20},
		ExistingDayCounts:   map[string]int{},
		MaxShiftsPerDay:     1,
		TargetDate:          tomorrow,
		Registrations:       regs,
	}

	solution := SolveOptimizedSchedule(input)
	if len(solution[1]) != 1 {
		t.Fatalf("Expected 1 worker assigned to shift 1, got %v", solution[1])
	}

	assignedUser := solution[1][0]
	if assignedUser != "alice" {
		t.Fatalf("Expected Alice to be scheduled for tomorrow based on registration arrival, but got %s", assignedUser)
	}
}

