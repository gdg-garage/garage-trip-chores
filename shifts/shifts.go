package shifts

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type UserFairnessMetric struct {
	UserId          string  `json:"user_id"`
	Name            string  `json:"name"`
	Handle          string  `json:"handle"`
	ChoreMin        float64 `json:"chore_min"`
	ChoreCount      float64 `json:"chore_count"`
	ShiftStandbyMin float64 `json:"shift_standby_min"`
	ShiftCount      float64 `json:"shift_count"`
	TotalMin        float64 `json:"total_min"`
	PresentTicks    int     `json:"present_ticks"`
	NormalizedTotal float64 `json:"normalized_total"`
}

type ShiftsService struct {
	storage *storage.Storage
	logger  *slog.Logger
	conf    Config
}

func NewService(storage *storage.Storage, logger *slog.Logger, conf Config) *ShiftsService {
	return &ShiftsService{
		storage: storage,
		logger:  logger,
		conf:    conf,
	}
}

func parseTimeOfDay(baseDate time.Time, timeStr string, loc *time.Location) (time.Time, error) {
	parts := strings.Split(strings.TrimSpace(timeStr), ":")
	if len(parts) != 2 {
		return baseDate, fmt.Errorf("invalid time format: %s", timeStr)
	}
	hour, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return baseDate, fmt.Errorf("invalid time numbers: %s", timeStr)
	}
	return time.Date(baseDate.Year(), baseDate.Month(), baseDate.Day(), hour, min, 0, 0, loc), nil
}

// IsSchedulingActiveForDate returns true if targetDate falls within an active/scheduled trip event,
// has registered attendees arriving/departing on that date, or has participants currently marked present on site.
// If none of these conditions are met, shift scheduling is suppressed to avoid scheduling out-of-event shifts.
func (s *ShiftsService) IsSchedulingActiveForDate(targetDate time.Time) (bool, string, error) {
	// 1. Check if date falls into any known trip event
	inEvent, event, err := s.storage.IsDateInEvent(targetDate)
	if err == nil && inEvent && event != nil {
		return true, fmt.Sprintf("Event: %s (%s)", event.Name, event.Code), nil
	}

	// 2. Check if any attendee registration covers targetDate
	regs, err := s.storage.GetUserRegistrationsMap(s.conf.Event)
	if err == nil && len(regs) > 0 {
		y, m, d := targetDate.Date()
		tDay := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		for _, r := range regs {
			if !r.Cancelled && !r.ArrivalDate.IsZero() && !r.DepartureDate.IsZero() {
				ay, am, ad := r.ArrivalDate.Date()
				arrDay := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
				dy, dm, dd := r.DepartureDate.Date()
				depDay := time.Date(dy, dm, dd, 23, 59, 59, 0, time.UTC)
				if !tDay.Before(arrDay) && !tDay.After(depDay) {
					return true, fmt.Sprintf("Attendee registration present (%s)", r.Username), nil
				}
			}
		}
	}

	// 3. Fallback only if no trip events are defined (e.g. headless tests/simulations)
	events, _ := s.storage.GetTripEvents()
	if len(events) == 0 {
		presentUsers, err := s.storage.GetPresentUsers()
		if err == nil && len(presentUsers) > 0 {
			return true, fmt.Sprintf("%d participants currently present on site", len(presentUsers)), nil
		}
		users, err := s.storage.GetUsers()
		if err == nil {
			presentCount := 0
			for _, u := range users {
				if u.IsPresent {
					presentCount++
				}
			}
			if presentCount > 0 {
				return true, fmt.Sprintf("%d participants present", presentCount), nil
			}
		}
	}

	return false, "", nil
}

// GenerateShiftsForDate creates scheduled shift instances based on active shift definitions
// and runs the ILP optimization solver to assign attendees fairly.
func (s *ShiftsService) GenerateShiftsForDate(targetDate time.Time, freeze bool) ([]storage.Shift, error) {
	loc := targetDate.Location()
	dateStr := targetDate.Format("2006-01-02")

	active, reason, err := s.IsSchedulingActiveForDate(targetDate)
	if err != nil {
		s.logger.Warn("Error checking if scheduling is active for date", "date", dateStr, "error", err)
	}
	if !active {
		s.logger.Info("No active event, registered attendees, or present users for date; skipping shift generation", "date", dateStr)
		return nil, nil
	}

	s.logger.Info("Generating shifts for date", "date", dateStr, "freeze", freeze, "reason", reason)

	defs, err := s.storage.GetActiveShiftDefinitions()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active shift definitions: %w", err)
	}

	// 1. Ensure Shift records exist for each slot defined by definitions
	for _, def := range defs {
		startTime, err1 := parseTimeOfDay(targetDate, def.ActiveHoursStart, loc)
		endTime, err2 := parseTimeOfDay(targetDate, def.ActiveHoursEnd, loc)
		if err1 != nil || err2 != nil || !endTime.After(startTime) {
			s.logger.Warn("Invalid active hours in definition", "def_id", def.ID, "start", def.ActiveHoursStart, "end", def.ActiveHoursEnd)
			continue
		}

		shiftLen := time.Duration(def.ShiftLengthMin) * time.Minute
		if shiftLen <= 0 {
			shiftLen = 120 * time.Minute
		}

		curStart := startTime
		for curStart.Add(shiftLen).Before(endTime) || curStart.Add(shiftLen).Equal(endTime) {
			curEnd := curStart.Add(shiftLen)

			// Check if this slot already exists
			existingShifts, err := s.storage.GetShifts(dateStr)
			if err != nil {
				return nil, err
			}

			exists := false
			for _, es := range existingShifts {
				if es.DefinitionID == def.ID && es.StartTime.Equal(curStart) {
					exists = true
					break
				}
			}

			if !exists {
				ratio := def.StandbyRatio
				if ratio <= 0 {
					ratio = s.conf.DefaultStandbyRatio
				}
				newShift := storage.Shift{
					DefinitionID: def.ID,
					Name:         def.Name,
					Category:     def.Category,
					DocURL:       def.DocURL,
					Date:         dateStr,
					StartTime:    curStart,
					EndTime:      curEnd,
					WorkersCount: def.WorkersCount,
					StandbyRatio: ratio,
					Frozen:       false,
					Status:       "scheduled",
				}
				_, err := s.storage.CreateShift(newShift)
				if err != nil {
					s.logger.Error("Failed to create shift slot", "error", err, "def_id", def.ID, "start", curStart)
				}
			}

			curStart = curEnd
		}
	}

	// 2. Fetch all shifts for the date
	shifts, err := s.storage.GetShifts(dateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to reload shifts: %w", err)
	}

	// 3. Find shifts that need worker assignments
	var unfilledShifts []storage.Shift
	existingDayCounts := make(map[string]int)

	for _, sh := range shifts {
		// Populate definitions if empty
		if sh.Definition.ID == 0 {
			if def, err := s.storage.GetShiftDefinition(sh.DefinitionID); err == nil && def != nil {
				sh.Definition = *def
			}
		}

		activeAssigned := 0
		for _, a := range sh.Assignments {
			if a.Status != "dropped" && a.Status != "refused" && a.Status != "swapped" {
				activeAssigned++
				existingDayCounts[a.UserId]++
			}
		}
		if uint(activeAssigned) < sh.WorkersCount {
			unfilledShifts = append(unfilledShifts, sh)
		}
	}

	// 4. Run Optimization Solver if there are slots to fill
	if len(unfilledShifts) > 0 {
		users, err := s.storage.GetUsers()
		if err != nil {
			return nil, fmt.Errorf("failed to get users: %w", err)
		}

		normStats, err := s.storage.GetTotalNormalizedChoreStats()
		if err != nil {
			return nil, fmt.Errorf("failed to get normalized stats: %w", err)
		}

		normMap := make(map[string]float64)
		for u, st := range normStats {
			normMap[u] = st.TotalMin
		}

		presenceCounts, _ := s.storage.GetUsersPresenceCounts()
		regMap, _ := s.storage.GetUserRegistrationsMap(s.conf.Event)

		input := SolverInput{
			Users:               users,
			Shifts:              unfilledShifts,
			NormalizedWorkloads: normMap,
			PresentTicks:        presenceCounts,
			ExistingDayCounts:   existingDayCounts,
			MaxShiftsPerDay:     s.conf.MaxShiftsPerPersonPerDay,
			TargetDate:          targetDate,
			Registrations:       regMap,
		}

		solution := SolveOptimizedSchedule(input)

		for shiftID, assignedUserIDs := range solution {
			// Find shift to calculate standby credits
			var targetShift *storage.Shift
			for i := range shifts {
				if shifts[i].ID == shiftID {
					targetShift = &shifts[i]
					break
				}
			}
			if targetShift == nil {
				continue
			}

			for _, uid := range assignedUserIDs {
				assignment := storage.ShiftAssignment{
					ShiftID:            shiftID,
					UserId:             uid,
					StandbyCreditedMin: targetShift.StandbyCreditedMin(),
					Status:             "assigned",
				}
				_, err := s.storage.CreateShiftAssignment(assignment)
				if err != nil {
					s.logger.Error("Failed to save shift assignment", "error", err, "shift_id", shiftID, "user_id", uid)
				}
			}
		}
	}

	// 5. Freeze if requested
	if freeze {
		if err := s.storage.FreezeShiftsForDate(dateStr); err != nil {
			s.logger.Error("Failed to freeze shifts for date", "date", dateStr, "error", err)
		}
	}

	// Return updated list
	return s.storage.GetShifts(dateStr)
}

// CanAttendShift marks an attendee's assignment as dropped and automatically
// reassigns the slot to the next fairest available attendee.
func (s *ShiftsService) CanAttendShift(assignmentID uint, reason string) (*storage.ShiftAssignment, error) {
	assignmentPtr, err := s.storage.GetShiftAssignment(assignmentID)
	if err != nil || assignmentPtr == nil {
		return nil, fmt.Errorf("assignment not found")
	}
	assignment := *assignmentPtr

	shift := assignment.Shift
	droppedUser := assignment.UserId

	// Mark as dropped
	assignment.Status = "dropped"
	assignment.Notes = reason
	if err := s.storage.UpdateShiftAssignment(assignment); err != nil {
		return nil, fmt.Errorf("failed to drop assignment: %w", err)
	}

	s.logger.Info("User cannot attend shift; seeking replacement", "user_id", droppedUser, "shift_id", shift.ID)

	// Seek replacement using solver
	users, err := s.storage.GetUsers()
	if err != nil {
		return nil, err
	}

	normStats, err := s.storage.GetTotalNormalizedChoreStats()
	if err != nil {
		return nil, err
	}
	normMap := make(map[string]float64)
	for u, st := range normStats {
		normMap[u] = st.TotalMin
	}
	presenceCounts, _ := s.storage.GetUsersPresenceCounts()

	// Shifts already assigned to people today
	existingCounts := make(map[string]int)
	dateShifts, _ := s.storage.GetShifts(shift.Date)
	for _, ds := range dateShifts {
		for _, a := range ds.Assignments {
			if a.Status != "dropped" && a.Status != "refused" && a.Status != "swapped" {
				existingCounts[a.UserId]++
			}
		}
	}

	// Filter out the user who just dropped
	candidateUsers := make([]storage.User, 0, len(users))
	for _, u := range users {
		if u.DiscordId != droppedUser {
			candidateUsers = append(candidateUsers, u)
		}
	}

	// Construct single shift solver input
	singleShift := shift
	singleShift.WorkersCount = 1 // only replacing 1 dropped worker
	regMap, _ := s.storage.GetUserRegistrationsMap(s.conf.Event)
	input := SolverInput{
		Users:               candidateUsers,
		Shifts:              []storage.Shift{singleShift},
		NormalizedWorkloads: normMap,
		PresentTicks:        presenceCounts,
		ExistingDayCounts:   existingCounts,
		MaxShiftsPerDay:     s.conf.MaxShiftsPerPersonPerDay + 1, // allow slight relaxation for emergency fill
		TargetDate:          shift.StartTime,
		Registrations:       regMap,
	}

	solution := SolveOptimizedSchedule(input)
	if uids, ok := solution[shift.ID]; ok && len(uids) > 0 {
		newUID := uids[0]
		newAssignment := storage.ShiftAssignment{
			ShiftID:            shift.ID,
			UserId:             newUID,
			StandbyCreditedMin: shift.StandbyCreditedMin(),
			Status:             "assigned",
			Notes:              fmt.Sprintf("Replacement for %s", droppedUser),
		}
		created, err := s.storage.CreateShiftAssignment(newAssignment)
		if err != nil {
			return nil, fmt.Errorf("failed to save replacement assignment: %w", err)
		}
		s.logger.Info("Replacement assigned for shift", "shift_id", shift.ID, "new_user_id", newUID)
		return &created, nil
	}

	return nil, errors.New("no eligible replacement available")
}

// RequestSwap creates a pending shift swap request
func (s *ShiftsService) RequestSwap(assignmentID uint, targetUserID string) (*storage.ShiftSwapRequest, error) {
	assignment, err := s.storage.GetShiftAssignment(assignmentID)
	if err != nil || assignment == nil {
		return nil, fmt.Errorf("assignment not found")
	}

	req := storage.ShiftSwapRequest{
		FromAssignmentID: assignmentID,
		FromUserId:       assignment.UserId,
		ToUserId:         targetUserID,
		Status:           "pending",
	}

	created, err := s.storage.CreateShiftSwapRequest(req)
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// AcceptSwap accepts a swap request and exchanges or reassigns the shift
func (s *ShiftsService) AcceptSwap(swapID uint, respondingUserID string) error {
	req, err := s.storage.GetShiftSwapRequest(swapID)
	if err != nil || req == nil {
		return fmt.Errorf("swap request not found")
	}
	if req.ToUserId != respondingUserID {
		return fmt.Errorf("not authorized to accept this swap request")
	}
	if req.Status != "pending" {
		return fmt.Errorf("swap request already %s", req.Status)
	}

	fromAssignment, err := s.storage.GetShiftAssignment(req.FromAssignmentID)
	if err != nil || fromAssignment == nil {
		return fmt.Errorf("from assignment not found")
	}

	// Update original assignment
	fromAssignment.Status = "swapped"
	_ = s.storage.UpdateShiftAssignment(*fromAssignment)

	// Create new assignment for the target user
	shift, err := s.storage.GetShift(fromAssignment.ShiftID)
	if err != nil || shift == nil {
		return fmt.Errorf("shift not found")
	}

	newAssignment := storage.ShiftAssignment{
		ShiftID:            shift.ID,
		UserId:             respondingUserID,
		StandbyCreditedMin: shift.StandbyCreditedMin(),
		Status:             "assigned",
		Notes:              fmt.Sprintf("Swapped from %s", fromAssignment.UserId),
	}
	_, err = s.storage.CreateShiftAssignment(newAssignment)
	if err != nil {
		return err
	}

	now := time.Now()
	req.Status = "accepted"
	req.ResolvedAt = &now
	return s.storage.UpdateShiftSwapRequest(*req)
}

// CompleteShift finalizes a shift and confirms standby credited minutes
func (s *ShiftsService) CompleteShift(shiftID uint) error {
	shift, err := s.storage.GetShift(shiftID)
	if err != nil || shift == nil {
		return fmt.Errorf("shift not found")
	}

	shift.Status = "completed"
	if err := s.storage.UpdateShift(*shift); err != nil {
		return err
	}

	for _, a := range shift.Assignments {
		if a.Status == "assigned" {
			a.Status = "completed"
			a.StandbyCreditedMin = shift.StandbyCreditedMin()
			_ = s.storage.UpdateShiftAssignment(a)
		}
	}

	return nil
}

// GetFairnessOverview generates a transparent metrics table comparing chore and shift contributions
func (s *ShiftsService) GetFairnessOverview() (map[string]UserFairnessMetric, error) {
	aggStats, err := s.storage.GetAggregatedStats()
	if err != nil {
		return nil, err
	}

	users, _ := s.storage.GetUsers()
	userMap := make(map[string]storage.User)
	for _, u := range users {
		userMap[u.DiscordId] = u
	}

	result := make(map[string]UserFairnessMetric)
	for uid, stats := range aggStats {
		u := userMap[uid]
		name := u.Name
		if name == "" {
			name = u.Handle
		}
		if name == "" {
			name = uid
		}

		result[uid] = UserFairnessMetric{
			UserId:          uid,
			Name:            name,
			Handle:          u.Handle,
			ChoreMin:        stats.WorkedMin,
			ChoreCount:      stats.WorkedCount,
			ShiftStandbyMin: stats.ShiftStandbyMin,
			ShiftCount:      stats.ShiftCount,
			TotalMin:        stats.TotalMin,
			PresentTicks:    stats.PresentTicks,
			NormalizedTotal: stats.NormalizedTotal,
		}
	}

	return result, nil
}
