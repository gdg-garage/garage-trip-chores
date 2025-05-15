package chores

type ChoreStats struct {
	Count    int
	TotalMin int
}

type UserChoreStats map[string]ChoreStats
