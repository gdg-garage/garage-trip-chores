package chores

import (
	"reflect"
	"testing"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func TestUserChoreStats(t *testing.T) {
	tests := []struct {
		name     string
		st       storage.UserChoreStats
		other    storage.UserChoreStats
		expected storage.UserChoreStats
	}{
		{
			name: "Add stats for different users",
			st: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
			},
			other: storage.UserChoreStats{
				"user2": {Count: 3, TotalMin: 45},
			},
			expected: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
				"user2": {Count: 3, TotalMin: 45},
			},
		},
		{
			name: "Add stats for the same user",
			st: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
			},
			other: storage.UserChoreStats{
				"user1": {Count: 3, TotalMin: 45},
			},
			expected: storage.UserChoreStats{
				"user1": {Count: 5, TotalMin: 75},
			},
		},
		{
			name: "Add stats with overlapping and new users",
			st: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
				"user2": {Count: 1, TotalMin: 15},
			},
			other: storage.UserChoreStats{
				"user2": {Count: 3, TotalMin: 45},
				"user3": {Count: 4, TotalMin: 60},
			},
			expected: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
				"user2": {Count: 4, TotalMin: 60},
				"user3": {Count: 4, TotalMin: 60},
			},
		},
		{
			name: "Add empty stats",
			st: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
			},
			other: storage.UserChoreStats{},
			expected: storage.UserChoreStats{
				"user1": {Count: 2, TotalMin: 30},
			},
		},
		{
			name: "Add to empty stats",
			st:   storage.UserChoreStats{},
			other: storage.UserChoreStats{
				"user1": {Count: 3, TotalMin: 45},
			},
			expected: storage.UserChoreStats{
				"user1": {Count: 3, TotalMin: 45},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.st.Add(tt.other)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
