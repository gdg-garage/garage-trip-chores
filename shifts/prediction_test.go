package shifts

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func TestPredictEventSchedule(t *testing.T) {
	st, cleanup := setupTestStorage(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	conf := DefaultConfig()
	service := NewService(st, logger, conf)

	// 1. Setup Trip Event: Day 1 (today) to Day 4 (today + 3 days)
	loc, _ := time.LoadLocation(conf.Timezone)
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	y, m, d := now.Date()
	start := time.Date(y, m, d, 10, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 3)

	event := storage.TripEvent{
		Code:      "gt-test",
		Name:      "Garage Trip Test",
		StartDate: &start,
		EndDate:   &end,
		Enabled:   true,
	}
	if err := st.SaveTripEvents([]storage.TripEvent{event}); err != nil {
		t.Fatalf("Failed to save event: %v", err)
	}

	// 2. Setup Shift Definitions: Kitchen (08:00-12:00, 2h slots) and Bar (18:00-22:00, 2h slots)
	_, err := st.CreateShiftDefinition(storage.ShiftDefinition{
		Name:             "Kitchen Duty",
		Category:         "Kitchen",
		WorkersCount:     1,
		ShiftLengthMin:   120,
		ActiveHoursStart: "08:00",
		ActiveHoursEnd:   "12:00",
		StandbyRatio:     0.10,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create kitchen def: %v", err)
	}

	_, err = st.CreateShiftDefinition(storage.ShiftDefinition{
		Name:             "Bar Steward",
		Category:         "Bar",
		WorkersCount:     1,
		ShiftLengthMin:   120,
		ActiveHoursStart: "18:00",
		ActiveHoursEnd:   "22:00",
		StandbyRatio:     0.10,
		Enabled:          true,
	})
	if err != nil {
		t.Fatalf("Failed to create bar def: %v", err)
	}

	// 3. Setup Users & Registrations:
	// Alice stays all 4 days
	// Bob arrives Day 2
	// Charlie departs Day 2
	// Dave stays all 4 days
	users := []storage.User{
		{DiscordId: "u_alice", Name: "Alice", Handle: "alice", IsPresent: true},
		{DiscordId: "u_bob", Name: "Bob", Handle: "bob", IsPresent: true},
		{DiscordId: "u_charlie", Name: "Charlie", Handle: "charlie", IsPresent: true},
		{DiscordId: "u_dave", Name: "Dave", Handle: "dave", IsPresent: true},
	}
	for _, u := range users {
		_ = st.SaveUser(u)
	}

	d1 := start
	d2 := start.AddDate(0, 0, 1)
	d4 := start.AddDate(0, 0, 3)

	regs := []storage.UserRegistration{
		{DiscordId: "u_alice", Username: "Alice", Event: "gt-test", ArrivalDate: d1, DepartureDate: d4},
		{DiscordId: "u_bob", Username: "Bob", Event: "gt-test", ArrivalDate: d2, DepartureDate: d4},
		{DiscordId: "u_charlie", Username: "Charlie", Event: "gt-test", ArrivalDate: d1, DepartureDate: d2},
		{DiscordId: "u_dave", Username: "Dave", Event: "gt-test", ArrivalDate: d1, DepartureDate: d4},
	}
	if err := st.SaveUserRegistrations(regs); err != nil {
		t.Fatalf("Failed to save registrations: %v", err)
	}

	// 4. Setup Day 1 as frozen in database
	d1Shifts, err := service.GenerateShiftsForDate(start, true)
	if err != nil {
		t.Fatalf("Failed to generate and freeze Day 1 shifts: %v", err)
	}
	if len(d1Shifts) == 0 {
		t.Fatalf("Expected Day 1 shifts to be generated")
	}

	// 5. Run PredictEventSchedule
	pred, err := service.PredictEventSchedule("gt-test")
	if err != nil {
		t.Fatalf("PredictEventSchedule failed: %v", err)
	}

	if pred.EventCode != "gt-test" {
		t.Errorf("Expected event code gt-test, got %s", pred.EventCode)
	}

	if len(pred.Days) != 4 {
		t.Fatalf("Expected 4 calendar days, got %d", len(pred.Days))
	}

	// Verify Day 1 is frozen
	day1 := pred.Days[0]
	if day1.Date != start.Format("2006-01-02") {
		t.Errorf("Expected Day 1 date %s, got %s", start.Format("2006-01-02"), day1.Date)
	}
	if len(day1.Shifts) == 0 {
		t.Errorf("Expected Day 1 shifts")
	}
	for _, s := range day1.Shifts {
		if s.Predicted {
			t.Errorf("Day 1 shifts should not be marked as predicted (they are frozen in DB)")
		}
	}

	// Verify Bob is NOT in Day 1 attendees (he arrives on Day 2)
	for _, a := range day1.Attendees {
		if a.DiscordID == "u_bob" {
			t.Errorf("Bob should not be eligible on Day 1 (arriving Day 2)")
		}
	}

	// Verify Day 3 (today + 2 days) is predicted
	day3 := pred.Days[2]
	day3Date := start.AddDate(0, 0, 2).Format("2006-01-02")
	if day3.Date != day3Date {
		t.Errorf("Expected Day 3 date %s, got %s", day3Date, day3.Date)
	}
	if len(day3.Shifts) == 0 {
		t.Errorf("Expected Day 3 predicted shifts")
	}
	for _, s := range day3.Shifts {
		if !s.Predicted {
			t.Errorf("Day 3 shifts should be marked as predicted")
		}
		if s.Status != "predicted" {
			t.Errorf("Expected Day 3 shift status 'predicted', got %s", s.Status)
		}
	}

	// Charlie departed on Oct 17, so he should NOT be in Day 3 (Oct 18) attendees
	for _, a := range day3.Attendees {
		if a.DiscordID == "u_charlie" {
			t.Errorf("Charlie should not be eligible on Day 3 (departed Oct 17)")
		}
	}
	// Bob arrived Oct 17, so he SHOULD be in Day 3 attendees
	bobFound := false
	for _, a := range day3.Attendees {
		if a.DiscordID == "u_bob" {
			bobFound = true
			break
		}
	}
	if !bobFound {
		t.Errorf("Bob should be eligible on Day 3")
	}
}
