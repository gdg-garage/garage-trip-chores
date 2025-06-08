package chores

import (
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func sliceIntersect(a, b []string) []string {
	// Find the intersection of two slices using a map for better performance.
	set := make(map[string]struct{})
	for _, item := range a {
		set[item] = struct{}{}
	}

	intersection := []string{}
	for _, item := range b {
		if _, found := set[item]; found {
			intersection = append(intersection, item)
		}
	}
	return intersection
}

func SortUsersBasedOnChoreStats(stats map[string]storage.ChoreStatsWithCapabilities) []string {
	// Sort users based on the number capabilities matched, chores total time and count in this order (lowest first).
	sortedUsers := make([]string, 0, len(stats))
	for user := range stats {
		sortedUsers = append(sortedUsers, user)
	}
	sort.Slice(sortedUsers, func(i, j int) bool {
		if stats[sortedUsers[i]].CapabilitiesMatched != stats[sortedUsers[j]].CapabilitiesMatched {
			return stats[sortedUsers[i]].CapabilitiesMatched < stats[sortedUsers[j]].CapabilitiesMatched
		}
		if stats[sortedUsers[i]].TotalMin != stats[sortedUsers[j]].TotalMin {
			return stats[sortedUsers[i]].TotalMin < stats[sortedUsers[j]].TotalMin
		}
		return stats[sortedUsers[i]].Count < stats[sortedUsers[j]].Count
	})
	return sortedUsers
}

func OversampleCnt(needed uint, ratio float64) uint {
	// Oversample the users based on the given ratio.
	// For example, if ratio is 0.5 and there are required 4 users, we will return 2 users.
	// If ratio is 1.0, we will return 4 users.
	// If ratio is 1.0, we will return 8 users.
	// If ratio is 0.0, we will return 0.
	// If ratio is 0.5, for 5 users we will return 3 users.
	if ratio <= 0 {
		return 0
	}
	return uint(math.Ceil(float64(needed) * ratio))
}

type ChoresLogic struct {
	storage *storage.Storage
	logger  *slog.Logger
	config  Config
}

func NewChoresLogic(storage *storage.Storage, logger *slog.Logger, config Config) ChoresLogic {
	return ChoresLogic{
		storage: storage,
		logger:  logger,
		config:  config,
	}
}

func (cl ChoresLogic) AssignChoresToUsers(users []storage.User, chore storage.Chore) ([]storage.ChoreAssignment, error) {
	needed := chore.NecessaryWorkers + OversampleCnt(chore.NecessaryWorkers, cl.config.OversampleRatio)
	assignments := make([]storage.ChoreAssignment, 0, needed)

	userTotalStats, err := cl.storage.GetTotalChoreStats()
	if err != nil {
		return assignments, err
	}

	userStatsWithCap := map[string]storage.ChoreStatsWithCapabilities{}
	for _, user := range users {
		if s, ok := userTotalStats[user.DiscordId]; ok {
			userStatsWithCap[user.DiscordId] = storage.ChoreStatsWithCapabilities{
				ChoreStats:          s,
				CapabilitiesMatched: uint(len(sliceIntersect(user.Capabilities, chore.GetCapabilities()))),
			}
		} else {
			userStatsWithCap[user.DiscordId] = storage.ChoreStatsWithCapabilities{
				ChoreStats: storage.ChoreStats{
					Count:    0,
					TotalMin: 0,
				},
				CapabilitiesMatched: uint(len(sliceIntersect(user.Capabilities, chore.GetCapabilities()))),
			}
		}
	}

	sortedUsers := SortUsersBasedOnChoreStats(userStatsWithCap)
	selectedUsers := sortedUsers[:int(math.Min(float64(len(sortedUsers)), float64(needed)))]

	// Create assignments for the selected users
	for _, user := range selectedUsers {
		assignment := storage.ChoreAssignment{
			UserId:  user,
			ChoreId: chore.ID,
			Chore:   chore,
			Created: time.Now(),
		}
		assignments = append(assignments, assignment)
	}
	return cl.storage.SaveChoreAssignments(assignments)
}
