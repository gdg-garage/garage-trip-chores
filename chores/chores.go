package chores

import "sort"

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
