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
