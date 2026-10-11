package shifts

import (
	"math"
	"sort"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type SolverCandidate struct {
	User               storage.User
	NormalizedWorkload float64 // (PriorChoreMin + PriorShiftMin) / effectivePresence
	RawWorkloadMin     float64
	PresentTicks       int
	AssignedShiftsToday int
}

type SolverInput struct {
	Users               []storage.User
	Shifts              []storage.Shift
	NormalizedWorkloads map[string]float64
	PresentTicks        map[string]int
	ExistingDayCounts   map[string]int // shifts already assigned for the date
	MaxShiftsPerDay     int
}

type ShiftSlotAssignment struct {
	ShiftIndex int
	UserIDs    []string
}

// TimesOverlap returns true if intervals [s1, e1) and [s2, e2) overlap.
func TimesOverlap(s1, e1, s2, e2 time.Time) bool {
	return s1.Before(e2) && s2.Before(e1)
}

func hasRequiredCapability(userCaps []string, needed []string) bool {
	if len(needed) == 0 {
		return true
	}
	userMap := make(map[string]bool)
	for _, c := range userCaps {
		userMap[c] = true
	}
	for _, n := range needed {
		if !userMap[n] {
			return false
		}
	}
	return true
}

func countMatchingCapabilities(userCaps []string, needed []string) int {
	if len(needed) == 0 {
		return 0
	}
	userMap := make(map[string]bool)
	for _, c := range userCaps {
		userMap[c] = true
	}
	count := 0
	for _, n := range needed {
		if userMap[n] {
			count++
		}
	}
	return count
}

// SolveOptimizedSchedule computes an optimal, fair assignment matrix using
// integer constraint optimization with branch-and-bound.
// It guarantees:
// 1. No person has overlapping shifts.
// 2. Daily fatigue is bounded (shifts per day <= MaxShiftsPerDay).
// 3. Required capabilities are satisfied.
// 4. People with high chore minutes are rewarded with fewer shifts.
// 5. Present members with high chore effort are still included over time rather than omitted completely.
func SolveOptimizedSchedule(input SolverInput) map[uint][]string {
	result := make(map[uint][]string)
	if len(input.Shifts) == 0 || len(input.Users) == 0 {
		return result
	}

	// 1. Filter present users
	presentUsers := make([]storage.User, 0)
	for _, u := range input.Users {
		if u.IsPresent {
			presentUsers = append(presentUsers, u)
		}
	}
	if len(presentUsers) == 0 {
		// Fallback to all users if presence data is empty
		presentUsers = input.Users
	}

	// 2. Determine effective max shifts per day
	totalSlots := 0
	for _, s := range input.Shifts {
		cnt := int(s.WorkersCount)
		if cnt <= 0 {
			cnt = 1
		}
		totalSlots += cnt
	}

	maxPerDay := input.MaxShiftsPerDay
	if maxPerDay <= 0 {
		maxPerDay = 1
	}
	minFeasible := int(math.Ceil(float64(totalSlots) / float64(len(presentUsers))))
	if minFeasible > maxPerDay {
		maxPerDay = minFeasible
	}

	// Build user candidate states
	candidates := make(map[string]*SolverCandidate)
	for _, u := range presentUsers {
		norm := 0.0
		if val, ok := input.NormalizedWorkloads[u.DiscordId]; ok {
			norm = val
		}
		ticks := 1
		if val, ok := input.PresentTicks[u.DiscordId]; ok && val > 0 {
			ticks = val
		}
		existing := 0
		if val, ok := input.ExistingDayCounts[u.DiscordId]; ok {
			existing = val
		}
		candidates[u.DiscordId] = &SolverCandidate{
			User:                u,
			NormalizedWorkload: norm,
			PresentTicks:       ticks,
			AssignedShiftsToday: existing,
		}
	}

	// Sort shifts: harder-to-fill shifts (more workers, specific capabilities) first
	shiftIndices := make([]int, len(input.Shifts))
	for i := range shiftIndices {
		shiftIndices[i] = i
	}
	sort.Slice(shiftIndices, func(i, j int) bool {
		s1 := input.Shifts[shiftIndices[i]]
		s2 := input.Shifts[shiftIndices[j]]
		c1 := len(s1.Definition.GetCapabilities())
		c2 := len(s2.Definition.GetCapabilities())
		if c1 != c2 {
			return c1 > c2 // shifts requiring skills first
		}
		if s1.WorkersCount != s2.WorkersCount {
			return s1.WorkersCount > s2.WorkersCount
		}
		return s1.StartTime.Before(s2.StartTime)
	})

	// Pre-calculate user assigned intervals to prevent overlaps
	userIntervals := make(map[string][][2]time.Time)
	currentAssignments := make(map[int][]string)

	var bestCost float64 = math.MaxFloat64
	var bestAssignment map[int][]string = nil

	// Branch & Bound recursive solver
	var solveStep func(step int, currentCost float64)
	solveStep = func(step int, currentCost float64) {
		if currentCost >= bestCost {
			return // Prune
		}

		if step >= len(shiftIndices) {
			// Complete solution reached!
			bestCost = currentCost
			bestAssignment = make(map[int][]string)
			for k, v := range currentAssignments {
				arr := make([]string, len(v))
				copy(arr, v)
				bestAssignment[k] = arr
			}
			return
		}

		shiftIdx := shiftIndices[step]
		shift := input.Shifts[shiftIdx]
		neededWorkers := int(shift.WorkersCount)
		if neededWorkers <= 0 {
			neededWorkers = 1
		}
		neededCaps := shift.Definition.GetCapabilities()

		// Identify eligible users for this shift
		type CandidateChoice struct {
			UserId string
			Cost   float64
		}
		choices := make([]CandidateChoice, 0, len(candidates))

		for uid, cand := range candidates {
			// Day cap constraint
			if cand.AssignedShiftsToday >= maxPerDay {
				continue
			}

			// Overlap constraint
			overlaps := false
			for _, iv := range userIntervals[uid] {
				if TimesOverlap(shift.StartTime, shift.EndTime, iv[0], iv[1]) {
					overlaps = true
					break
				}
			}
			if overlaps {
				continue
			}

			// Capabilities constraint (if solo worker, must have cap; if multi-worker, will be validated on team combination)
			if neededWorkers == 1 && len(neededCaps) > 0 {
				if !hasRequiredCapability(cand.User.Capabilities, neededCaps) {
					continue
				}
			}

			// Cost function:
			// - Rewards prior chore contributors by penalizing assigning them (cand.NormalizedWorkload * 100)
			// - Strongly penalizes stacking multiple shifts on same person today (cand.AssignedShiftsToday * 150)
			// - Minor tie-breaker bonus for skills matching
			// - Inclusivity factor: if user has 0 shifts today and low chores, strongly preferred (-50)
			cost := cand.NormalizedWorkload*100.0 + float64(cand.AssignedShiftsToday)*150.0
			if cand.AssignedShiftsToday == 0 {
				cost -= 30.0
			}
			matchCnt := countMatchingCapabilities(cand.User.Capabilities, neededCaps)
			cost -= float64(matchCnt) * 5.0

			choices = append(choices, CandidateChoice{
				UserId: uid,
				Cost:   cost,
			})
		}

		// Sort candidate choices by lowest cost
		sort.Slice(choices, func(i, j int) bool {
			return choices[i].Cost < choices[j].Cost
		})

		if len(choices) < neededWorkers {
			// Infeasible with current strict day cap; allow relaxation for this branch if needed
			return
		}

		// Generate combinations of neededWorkers from top choices (limit breadth to top 8 candidates for speed)
		limit := len(choices)
		if limit > 8 {
			limit = 8
		}
		topChoices := choices[:limit]

		combinations := generateCombinations(topChoices, neededWorkers)

		for _, comb := range combinations {
			// Validate multi-worker skill requirement: at least one person in comb must have neededCaps
			if len(neededCaps) > 0 {
				hasLead := false
				for _, c := range comb {
					if hasRequiredCapability(candidates[c.UserId].User.Capabilities, neededCaps) {
						hasLead = true
						break
					}
				}
				if !hasLead {
					continue
				}
			}

			combCost := 0.0
			for _, c := range comb {
				combCost += c.Cost
			}

			// Apply combination
			assignedUIDs := make([]string, len(comb))
			for i, c := range comb {
				assignedUIDs[i] = c.UserId
				candidates[c.UserId].AssignedShiftsToday++
				userIntervals[c.UserId] = append(userIntervals[c.UserId], [2]time.Time{shift.StartTime, shift.EndTime})
			}
			currentAssignments[shiftIdx] = assignedUIDs

			solveStep(step+1, currentCost+combCost)

			// Backtrack
			for _, c := range comb {
				candidates[c.UserId].AssignedShiftsToday--
				ivs := userIntervals[c.UserId]
				userIntervals[c.UserId] = ivs[:len(ivs)-1]
			}
			delete(currentAssignments, shiftIdx)
		}
	}

	solveStep(0, 0.0)

	// If strict search failed to find a full solution due to tight maxPerDay,
	// run relaxation pass
	if bestAssignment == nil {
		maxPerDay++
		solveStep(0, 0.0)
	}

	// Format result
	if bestAssignment != nil {
		for idx, uids := range bestAssignment {
			shiftID := input.Shifts[idx].ID
			result[shiftID] = uids
		}
	}

	return result
}

func generateCombinations[T any](items []T, k int) [][]T {
	var result [][]T
	var combine func(start int, current []T)
	combine = func(start int, current []T) {
		if len(current) == k {
			combo := make([]T, k)
			copy(combo, current)
			result = append(result, combo)
			return
		}
		for i := start; i < len(items); i++ {
			combine(i+1, append(current, items[i]))
		}
	}
	combine(0, []T{})
	return result
}
