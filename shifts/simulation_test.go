package shifts

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func setupTestStorage(t *testing.T) (*storage.Storage, func()) {
	tmpDir, err := os.MkdirTemp("", "shifts_sim_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "sim.db")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	conf := storage.Config{
		DbPath: dbPath,
	}

	s, err := storage.New(conf, logger)
	if err != nil {
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return s, cleanup
}

func TestMultiDayTripSimulation(t *testing.T) {
	s, cleanup := setupTestStorage(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	conf := DefaultConfig()
	service := NewService(s, logger, conf)

	// 1. Create 15 attendees
	for i := 1; i <= 15; i++ {
		uid := fmt.Sprintf("user_%02d", i)
		caps := []string{}
		if i <= 3 {
			caps = append(caps, "skill::kitchen")
		}
		user := storage.User{
			DiscordId:    uid,
			Name:         fmt.Sprintf("Attendee %02d", i),
			Handle:       fmt.Sprintf("handle_%02d", i),
			Capabilities: caps,
			IsPresent:    true,
		}
		_ = s.SaveUser(user)

		// Record presence ticks
		for p := 0; p < 10; p++ {
			_, _ = s.LogUserPresence(uid)
		}
	}

	// 2. Create 3 Shift Definitions
	kitchenDef, err := s.CreateShiftDefinition(storage.ShiftDefinition{
		Name:                  "Kitchen Duty",
		Category:              "Kitchen",
		DocURL:                "https://docs.google.com/document/d/kitchen-runbook",
		WorkersCount:          1,
		ShiftLengthMin:        120, // 2 hours
		ActiveHoursStart:      "08:00",
		ActiveHoursEnd:        "12:00", // 2 slots: 08:00-10:00, 10:00-12:00
		StandbyRatio:          0.10,
		NecessaryCapabilities: "skill::kitchen",
		Enabled:               true,
	})
	if err != nil {
		t.Fatalf("Failed to create kitchen def: %v", err)
	}

	barDef, err := s.CreateShiftDefinition(storage.ShiftDefinition{
		Name:             "Bar Steward",
		Category:         "Bar",
		DocURL:           "https://docs.google.com/document/d/bar-runbook",
		WorkersCount:     1,
		ShiftLengthMin:   120, // 2 hours
		ActiveHoursStart: "18:00",
		ActiveHoursEnd:   "22:00", // 2 slots: 18:00-20:00, 20:00-22:00
		StandbyRatio:     0.10,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create bar def: %v", err)
	}

	trashDef, err := s.CreateShiftDefinition(storage.ShiftDefinition{
		Name:             "Sanitation & Trash",
		Category:         "Facility",
		DocURL:           "https://docs.google.com/document/d/trash-runbook",
		WorkersCount:     1,
		ShiftLengthMin:   60, // 1 hour
		ActiveHoursStart: "14:00",
		ActiveHoursEnd:   "16:00", // 2 slots: 14:00-15:00, 15:00-16:00
		StandbyRatio:     0.10,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create trash def: %v", err)
	}

	t.Logf("Created shift definitions: Kitchen (%d), Bar (%d), Trash (%d)", kitchenDef.ID, barDef.ID, trashDef.ID)

	// Simulation: Day 1
	day1 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.Local)
	day1Shifts, err := service.GenerateShiftsForDate(day1, true)
	if err != nil {
		t.Fatalf("Failed to generate day 1 shifts: %v", err)
	}

	if len(day1Shifts) != 6 { // 2 kitchen + 2 bar + 2 trash = 6 shifts
		t.Fatalf("Expected 6 shifts on Day 1, got %d", len(day1Shifts))
	}

	// Verify all shifts on Day 1 are frozen (since freeze=true was requested at noon)
	for _, sh := range day1Shifts {
		if !sh.Frozen {
			t.Fatalf("Expected shift %d to be frozen", sh.ID)
		}
		if len(sh.Assignments) != 1 {
			t.Fatalf("Expected 1 assignment for shift %d, got %d", sh.ID, len(sh.Assignments))
		}
	}

	// In the afternoon of Day 1: Attendee 04 does a heavy ad-hoc chore (80 minutes cooking assistance)
	// Attendee 05 does a 40-minute firewood chore
	_, _ = s.SaveWorkLog(storage.WorkLog{
		UserId:       "user_04",
		TimeSpentMin: 80,
	})
	_, _ = s.SaveWorkLog(storage.WorkLog{
		UserId:       "user_05",
		TimeSpentMin: 40,
	})

	// Complete all Day 1 shifts (credits standby minutes)
	for _, sh := range day1Shifts {
		if err := service.CompleteShift(sh.ID); err != nil {
			t.Fatalf("Failed to complete shift %d: %v", sh.ID, err)
		}
	}

	// Simulation: Day 2 (Generated at noon for tomorrow)
	day2 := day1.AddDate(0, 0, 1)
	day2Shifts, err := service.GenerateShiftsForDate(day2, true)
	if err != nil {
		t.Fatalf("Failed to generate day 2 shifts: %v", err)
	}

	// Verify Day 2 schedule respects chore balance:
	// User 04 worked 80 min of chores! They should NOT be assigned multiple heavy shifts on Day 2.
	day2AssignmentsForUser04 := 0
	for _, sh := range day2Shifts {
		for _, a := range sh.Assignments {
			if a.UserId == "user_04" {
				day2AssignmentsForUser04++
			}
		}
	}
	if day2AssignmentsForUser04 > 1 {
		t.Errorf("User 04 did 80 min of chores but was assigned %d shifts on Day 2", day2AssignmentsForUser04)
	}

	// Test "Can't Attend" and Auto-Reschedule
	// Pick first assignment of Day 2
	firstAssign := day2Shifts[0].Assignments[0]
	originalUser := firstAssign.UserId

	replacement, err := service.CanAttendShift(firstAssign.ID, "Sick with headache")
	if err != nil {
		t.Fatalf("CanAttendShift failed: %v", err)
	}
	if replacement == nil || replacement.UserId == originalUser {
		t.Fatalf("Expected replacement to be a different attendee, got %v", replacement)
	}
	t.Logf("User %s dropped shift; successfully replaced by %s", originalUser, replacement.UserId)

	// Test Shift Swap
	secondAssign := day2Shifts[1].Assignments[0]
	targetUser := "user_15"
	swapReq, err := service.RequestSwap(secondAssign.ID, targetUser)
	if err != nil {
		t.Fatalf("RequestSwap failed: %v", err)
	}
	if swapReq.Status != "pending" {
		t.Fatalf("Expected swap request to be pending, got %s", swapReq.Status)
	}

	err = service.AcceptSwap(swapReq.ID, targetUser)
	if err != nil {
		t.Fatalf("AcceptSwap failed: %v", err)
	}

	// Complete Day 2 shifts
	for _, sh := range day2Shifts {
		_ = service.CompleteShift(sh.ID)
	}

	// Verify Fairness Overview
	overview, err := service.GetFairnessOverview()
	if err != nil {
		t.Fatalf("Failed to get fairness overview: %v", err)
	}

	totalAttending := len(overview)
	if totalAttending < 15 {
		t.Fatalf("Expected 15 attendees in fairness overview, got %d", totalAttending)
	}

	// Check that User 04 has their chore minutes accurately reported
	u4Metric := overview["user_04"]
	if u4Metric.ChoreMin != 80 {
		t.Errorf("Expected User 04 to have 80 chore min, got %f", u4Metric.ChoreMin)
	}

	t.Logf("Fairness overview verified for %d users successfully!", totalAttending)
}
