package chores

import (
	"math"
	"sort"
)

func SortUsersBasedOnChoreStats(stats UserChoreStats) []string {
	// Sort users based on the number of chores total time and count as secondary key.
	sortedUsers := make([]string, 0, len(stats))
	for user := range stats {
		sortedUsers = append(sortedUsers, user)
	}
	sort.Slice(sortedUsers, func(i, j int) bool {
		if stats[sortedUsers[i]].TotalMin != stats[sortedUsers[j]].TotalMin {
			return stats[sortedUsers[i]].TotalMin < stats[sortedUsers[j]].TotalMin
		}
		return stats[sortedUsers[i]].Count < stats[sortedUsers[j]].Count
	})
	return sortedUsers
}

func OversampleCnt(needed int, ratio float64) int {
	// Oversample the users based on the given ratio.
	// For example, if ratio is 0.5 and there are required 4 users, we will return 2 users.
	// If ratio is 1.0, we will return 4 users.
	// If ratio is 1.0, we will return 8 users.
	// If ratio is 0.0, we will return 0.
	// If ratio is 0.5, for 5 users we will return 3 users.
	if ratio <= 0 {
		return 0
	}
	return int(math.Ceil(float64(needed) * ratio))
}
