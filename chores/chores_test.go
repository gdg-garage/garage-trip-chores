package chores

import (
	"testing"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func TestUserOrderBasedOnStats(t *testing.T) {
	statistics := map[string]storage.ChoreStatsWithCapabilities{
		"user1": {
			ChoreStats: storage.ChoreStats{
				Count:    10,
				TotalMin: 30,
			},
			CapabilitiesMatched: 1,
		},
		"user2": {
			ChoreStats: storage.ChoreStats{
				Count:    5,
				TotalMin: 30,
			},
			CapabilitiesMatched: 2,
		},
		"user3": {
			ChoreStats: storage.ChoreStats{
				Count:    10,
				TotalMin: 40,
			},
			CapabilitiesMatched: 1,
		},
		"user4": {
			ChoreStats: storage.ChoreStats{
				Count:    4,
				TotalMin: 30,
			},
			CapabilitiesMatched: 1,
		},
		"user5": {
			ChoreStats: storage.ChoreStats{
				Count:    0,
				TotalMin: 0,
			},
			CapabilitiesMatched: 0,
		},
	}

	expectedOrder := []string{"user2", "user4", "user1", "user3", "user5"}

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
	statistics := map[string]storage.ChoreStatsWithCapabilities{}

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

func TestSortUsersBasedOnChoreStats_CooldownAndPresenceAndDeterministic(t *testing.T) {
	// Test 1: Cooldown deprioritizes user even if they have 0 chores
	statsWithCooldown := map[string]storage.ChoreStatsWithCapabilities{
		"alice": {
			ChoreStats:          storage.ChoreStats{Count: 0, TotalMin: 0},
			CapabilitiesMatched: 1,
			OnCooldown:          true,
		},
		"bob": {
			ChoreStats:          storage.ChoreStats{Count: 1, TotalMin: 10},
			CapabilitiesMatched: 1,
			OnCooldown:          false,
		},
	}
	// Bob is not on cooldown, so Bob should come before Alice
	sorted1 := SortUsersBasedOnChoreStats(statsWithCooldown)
	if len(sorted1) != 2 || sorted1[0] != "bob" || sorted1[1] != "alice" {
		t.Fatalf("Expected [bob, alice] due to cooldown, got %v", sorted1)
	}

	// Test 2: Presence ticks tie-break for users with equal 0 chores
	// Slacking for 50 ticks should be prioritized before someone present for 2 ticks
	statsWithPresence := map[string]storage.ChoreStatsWithCapabilities{
		"newcomer": {
			ChoreStats:          storage.ChoreStats{Count: 0, TotalMin: 0},
			CapabilitiesMatched: 1,
			PresentTicks:        2,
		},
		"slacker": {
			ChoreStats:          storage.ChoreStats{Count: 0, TotalMin: 0},
			CapabilitiesMatched: 1,
			PresentTicks:        50,
		},
	}
	sorted2 := SortUsersBasedOnChoreStats(statsWithPresence)
	if len(sorted2) != 2 || sorted2[0] != "slacker" || sorted2[1] != "newcomer" {
		t.Fatalf("Expected [slacker, newcomer] due to presence ticks, got %v", sorted2)
	}

	// Test 3: Deterministic tie-break by user ID when all stats are identical
	statsIdentical := map[string]storage.ChoreStatsWithCapabilities{
		"user_z": {ChoreStats: storage.ChoreStats{Count: 0, TotalMin: 0}, CapabilitiesMatched: 1, PresentTicks: 10},
		"user_a": {ChoreStats: storage.ChoreStats{Count: 0, TotalMin: 0}, CapabilitiesMatched: 1, PresentTicks: 10},
		"user_m": {ChoreStats: storage.ChoreStats{Count: 0, TotalMin: 0}, CapabilitiesMatched: 1, PresentTicks: 10},
	}
	sorted3 := SortUsersBasedOnChoreStats(statsIdentical)
	if len(sorted3) != 3 || sorted3[0] != "user_a" || sorted3[1] != "user_m" || sorted3[2] != "user_z" {
		t.Fatalf("Expected alphabetical tie-break [user_a, user_m, user_z], got %v", sorted3)
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
		got := OversampleCnt(uint(tt.needed), tt.ratio)
		if got != uint(tt.expected) {
			t.Errorf("OversampleCnt(%d, %.2f) = %d; want %d", tt.needed, tt.ratio, got, tt.expected)
		}
	}
}

func TestSliceIntersect(t *testing.T) {
	tests := []struct {
		a        []string
		b        []string
		expected []string
	}{
		{
			a:        []string{"apple", "banana", "cherry"},
			b:        []string{"banana", "cherry", "date"},
			expected: []string{"banana", "cherry"},
		},
		{
			a:        []string{"apple", "banana", "cherry"},
			b:        []string{"date", "fig", "grape"},
			expected: []string{},
		},
		{
			a:        []string{"apple", "banana", "cherry"},
			b:        []string{"apple", "banana", "cherry"},
			expected: []string{"apple", "banana", "cherry"},
		},
		{
			a:        []string{},
			b:        []string{"apple", "banana"},
			expected: []string{},
		},
		{
			a:        []string{"apple", "banana"},
			b:        []string{},
			expected: []string{},
		},
		{
			a:        []string{},
			b:        []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		got := sliceIntersect(tt.a, tt.b)
		if len(got) != len(tt.expected) {
			t.Errorf("sliceIntersect(%v, %v) = %v; want %v", tt.a, tt.b, got, tt.expected)
			continue
		}
		for i, v := range got {
			if v != tt.expected[i] {
				t.Errorf("sliceIntersect(%v, %v) = %v; want %v", tt.a, tt.b, got, tt.expected)
				break
			}
		}
	}
}
