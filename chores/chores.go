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

type CooldownStorageAccess interface {
	GetLastUserWorkTime(userId string) (*time.Time, error)
}

type PresenceStorageAccess interface {
	GetUsersPresenceCounts() (map[string]int, error)
}

func SortUsersBasedOnChoreStats(stats map[string]storage.ChoreStatsWithCapabilities) []string {
	// Sort users based on:
	// 1. OnCooldown: false before true (not on cooldown prioritized)
	// 2. CapabilitiesMatched: highest first
	// 3. TotalMin: lowest first
	// 4. Count: lowest first
	// 5. PresentTicks: highest first (if worked time & count are equal, people present longer get assigned first)
	// 6. User ID: alphabetical deterministic tie breaker
	sortedUsers := make([]string, 0, len(stats))
	for user := range stats {
		sortedUsers = append(sortedUsers, user)
	}
	sort.Slice(sortedUsers, func(i, j int) bool {
		u1 := sortedUsers[i]
		u2 := sortedUsers[j]
		s1 := stats[u1]
		s2 := stats[u2]

		if s1.OnCooldown != s2.OnCooldown {
			return !s1.OnCooldown
		}
		if s1.CapabilitiesMatched != s2.CapabilitiesMatched {
			return s1.CapabilitiesMatched > s2.CapabilitiesMatched
		}
		if s1.TotalMin != s2.TotalMin {
			return s1.TotalMin < s2.TotalMin
		}
		if s1.Count != s2.Count {
			return s1.Count < s2.Count
		}
		if s1.PresentTicks != s2.PresentTicks {
			return s1.PresentTicks > s2.PresentTicks
		}
		return u1 < u2
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

type StorageAccess interface {
	GetTotalNormalizedChoreStats() (storage.UserChoreStats, error)
	GetChoreAssignments(choreId uint) ([]storage.ChoreAssignment, error)
	SaveChoreAssignments(assignments []storage.ChoreAssignment) ([]storage.ChoreAssignment, error)
}

type ChoresLogic struct {
	storage StorageAccess
	logger  *slog.Logger
	config  Config
}

func NewChoresLogic(storage StorageAccess, logger *slog.Logger, config Config) ChoresLogic {
	return ChoresLogic{
		storage: storage,
		logger:  logger,
		config:  config,
	}
}

func (cl ChoresLogic) AssignChoresToUsers(users []storage.User, chore storage.Chore) ([]storage.ChoreAssignment, error) {
	needed := chore.NecessaryWorkers
	if chore.AssigneeId == "" {
		needed += OversampleCnt(chore.NecessaryWorkers, cl.config.OversampleRatio)
	}
	assignments := make([]storage.ChoreAssignment, 0, needed)

	userTotalStats, err := cl.storage.GetTotalNormalizedChoreStats()
	if err != nil {
		return assignments, err
	}

	var presenceCounts map[string]int
	if ps, ok := cl.storage.(PresenceStorageAccess); ok {
		if pc, err := ps.GetUsersPresenceCounts(); err == nil {
			presenceCounts = pc
		}
	}

	now := time.Now()
	userStatsWithCap := map[string]storage.ChoreStatsWithCapabilities{}
	for _, user := range users {
		var s storage.ChoreStats
		if st, ok := userTotalStats[user.DiscordId]; ok {
			s = st
		}

		onCooldown := false
		if cl.config.CooldownMin > 0 {
			if cs, ok := cl.storage.(CooldownStorageAccess); ok {
				if lastWork, err := cs.GetLastUserWorkTime(user.DiscordId); err == nil && lastWork != nil {
					if now.Sub(*lastWork) < time.Duration(cl.config.CooldownMin)*time.Minute {
						onCooldown = true
					}
				}
			}
		}

		presentTicks := 0
		if presenceCounts != nil {
			presentTicks = presenceCounts[user.DiscordId]
		}

		userStatsWithCap[user.DiscordId] = storage.ChoreStatsWithCapabilities{
			ChoreStats:          s,
			CapabilitiesMatched: uint(len(sliceIntersect(user.Capabilities, chore.GetCapabilities()))),
			PresentTicks:        presentTicks,
			OnCooldown:          onCooldown,
		}
	}

	alreadyAssignedCnt := uint(0)
	ass, err := cl.storage.GetChoreAssignments(chore.ID)
	if err != nil {
		cl.logger.Error("failed to get chore assignments", "error", err, "chore_id", chore.ID)
		return nil, err
	}
	for _, a := range ass {
		delete(userStatsWithCap, a.UserId)
		if a.Refused == nil && a.Timeouted == nil {
			alreadyAssignedCnt++
		}
	}

	if alreadyAssignedCnt >= needed {
		return assignments, nil
	}
	needed -= alreadyAssignedCnt

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
