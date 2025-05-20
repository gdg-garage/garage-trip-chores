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

	expectedOrder := []string{"user5", "user4", "user1", "user3", "user2"}

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
