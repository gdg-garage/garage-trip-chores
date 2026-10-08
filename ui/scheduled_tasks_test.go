package ui

import (
	"testing"
	"time"
)

func TestCronHelpers(t *testing.T) {
	loc := getTimeLocation()

	// Test ParseCronNext for 9:00 AM
	now := time.Date(2026, 10, 8, 8, 0, 0, 0, loc)
	next, err := ParseCronNext("0 9 * * *", now, loc)
	if err != nil {
		t.Fatalf("Failed to parse standard cron: %v", err)
	}
	expected := time.Date(2026, 10, 8, 9, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("Expected next run at %v, got %v", expected, next)
	}

	// Test after 9:00 AM (should run tomorrow at 9:00 AM)
	now = time.Date(2026, 10, 8, 10, 0, 0, 0, loc)
	next, err = ParseCronNext("0 9 * * *", now, loc)
	if err != nil {
		t.Fatalf("Failed to parse standard cron: %v", err)
	}
	expected = time.Date(2026, 10, 9, 9, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("Expected next run at %v, got %v", expected, next)
	}

	// Test invalid cron
	_, err = ParseCronNext("invalid-cron", now, loc)
	if err == nil {
		t.Fatal("Expected error for invalid cron syntax")
	}
}
