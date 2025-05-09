package chores

import (
	"testing"
)

func TestUserOrderBasedOnStats(t *testing.T) {
	statistics := UserChoreStats{
		"user1": ChoreStats{
			Count:    10,
			TotalMin: 30,
		},
		"user2": ChoreStats{
			Count:    5,
			TotalMin: 30,
		},
		"user3": ChoreStats{
			Count:    10,
			TotalMin: 40,
		},
	}

	expectedOrder := []string{"user2", "user1", "user3"}

	sortedUsers := SortUsersBasedOnChoreStats(statistics)

	if len(sortedUsers) != len(expectedOrder) {
		t.Fatalf("Expected sorted users length: %d, but got: %d", len(expectedOrder), len(sortedUsers))
	}

	for i, user := range sortedUsers {
		if user != expectedOrder[i] {
			t.Errorf("At index %d, expected user: %s, but got: %s", i, expectedOrder[i], user)
		}
	}
}
func TestEmptyStatsInput(t *testing.T) {
	statistics := UserChoreStats{}

	expectedOrder := []string{}

	sortedUsers := SortUsersBasedOnChoreStats(statistics)

	if len(sortedUsers) != len(expectedOrder) {
		t.Fatalf("Expected sorted users length: %d, but got: %d", len(expectedOrder), len(sortedUsers))
	}

	for i, user := range sortedUsers {
		if user != expectedOrder[i] {
			t.Errorf("At index %d, expected user: %s, but got: %s", i, expectedOrder[i], user)
		}
	}
}
func TestOversampleCnt(t *testing.T) {
	tests := []struct {
		needed   int
		ratio    float64
		expected int
	}{
		{4, 0.5, 2},
		{5, 0.5, 3},
		{4, 1.0, 4},
		{4, 2.0, 8},
		{8, 1.0, 8},
		{4, 0.0, 0},
		{0, 0.5, 0},
		{3, 0.33, 1},
		{3, 0.34, 2},
	}

	for _, tt := range tests {
		got := OversampleCnt(tt.needed, tt.ratio)
		if got != tt.expected {
			t.Errorf("OversampleCnt(%d, %.2f) = %d; want %d", tt.needed, tt.ratio, got, tt.expected)
		}
	}
}
