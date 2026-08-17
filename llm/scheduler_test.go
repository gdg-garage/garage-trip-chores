package llm

import (
	"testing"
	"time"
)

func TestNextRunDuration(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		loc = time.FixedZone("CET", 1*3600)
	}

	// 1. Morning at 10:00 CET -> next is 13:00 CET (3 hours)
	t10 := time.Date(2026, 8, 17, 10, 0, 0, 0, loc)
	d1 := NextRunDuration(t10, loc)
	if d1 != 3*time.Hour {
		t.Errorf("Expected 3h from 10:00 to 13:00, got %v", d1)
	}

	// 2. Afternoon at 14:00 CET -> next is 19:00 CET (5 hours)
	t14 := time.Date(2026, 8, 17, 14, 0, 0, 0, loc)
	d2 := NextRunDuration(t14, loc)
	if d2 != 5*time.Hour {
		t.Errorf("Expected 5h from 14:00 to 19:00, got %v", d2)
	}

	// 3. Evening at 20:00 CET -> next is 13:00 CET tomorrow (17 hours)
	t20 := time.Date(2026, 8, 17, 20, 0, 0, 0, loc)
	d3 := NextRunDuration(t20, loc)
	if d3 != 17*time.Hour {
		t.Errorf("Expected 17h from 20:00 to 13:00 tomorrow, got %v", d3)
	}
}
