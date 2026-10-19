package shifts

import (
	"fmt"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/robfig/cron/v3"
)

type PredictedAssignment struct {
	UserID             string  `json:"user_id"`
	Name               string  `json:"name"`
	Handle             string  `json:"handle"`
	StandbyCreditedMin float64 `json:"standby_credited_min"`
	Status             string  `json:"status"` // "assigned", "predicted", "completed", "dropped"
	Predicted          bool    `json:"predicted"`
}

type PredictedShift struct {
	ID           uint                  `json:"id,omitempty"`
	DefinitionID uint                  `json:"definition_id"`
	Name         string                `json:"name"`
	Category     string                `json:"category"`
	DocURL       string                `json:"doc_url,omitempty"`
	Date         string                `json:"date"`
	StartTime    time.Time             `json:"start_time"`
	EndTime      time.Time             `json:"end_time"`
	WorkersCount uint                  `json:"workers_count"`
	StandbyRatio float64               `json:"standby_ratio"`
	Frozen       bool                  `json:"frozen"`
	Predicted    bool                  `json:"predicted"`
	Status       string                `json:"status"` // "completed", "frozen", "predicted", "scheduled"
	Assignments  []PredictedAssignment `json:"assignments"`
}

type ScheduledChoreItem struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	EstimatedTimeMin uint     `json:"estimated_time_min"`
	NecessaryWorkers uint     `json:"necessary_workers"`
	CronExpr         string   `json:"cron_expr,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
}

type CalendarAttendee struct {
	DiscordID     string    `json:"discord_id"`
	Name          string    `json:"name"`
	Handle        string    `json:"handle"`
	ArrivalDate   time.Time `json:"arrival_date,omitempty"`
	DepartureDate time.Time `json:"departure_date,omitempty"`
	IsPresent     bool      `json:"is_present"`
}

type CalendarDaySchedule struct {
	Date            string               `json:"date"`           // YYYY-MM-DD
	DayName         string               `json:"day_name"`       // "Friday", etc.
	FormattedDate   string               `json:"formatted_date"` // "Fri, Oct 16"
	IsToday         bool                 `json:"is_today"`
	IsPast          bool                 `json:"is_past"`
	Status          string               `json:"status"` // "past", "frozen", "predicted", "idle"
	AttendeesCount  int                  `json:"attendees_count"`
	Attendees       []CalendarAttendee   `json:"attendees"`
	Shifts          []PredictedShift     `json:"shifts"`
	ScheduledChores []ScheduledChoreItem `json:"scheduled_chores"`
}

type EventSchedulePrediction struct {
	EventCode string                `json:"event_code"`
	EventName string                `json:"event_name"`
	StartDate time.Time             `json:"start_date"`
	EndDate   time.Time             `json:"end_date"`
	Events    []storage.TripEvent   `json:"events"`
	Days      []CalendarDaySchedule `json:"days"`
}

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// PredictEventSchedule returns a complete calendar schedule for an event spanning its full date range.
// For past or frozen days, it reflects the actual locked schedule and assignments.
// For upcoming unfrozen days, it simulates the ILP solver day-by-day based on current stats,
// registered attendees, and scheduled chores, projecting future shift assignments fairly.
func (s *ShiftsService) PredictEventSchedule(eventCode string) (*EventSchedulePrediction, error) {
	events, err := s.storage.GetTripEvents()
	if err != nil {
		s.logger.Warn("Failed to fetch trip events", "error", err)
	}

	var targetEvent *storage.TripEvent
	if eventCode != "" {
		for i := range events {
			if events[i].Code == eventCode {
				targetEvent = &events[i]
				break
			}
		}
	}
	if targetEvent == nil && s.conf.Event != "" {
		for i := range events {
			if events[i].Code == s.conf.Event {
				targetEvent = &events[i]
				break
			}
		}
	}
	if targetEvent == nil {
		now := time.Now()
		for i := range events {
			if events[i].StartDate != nil && events[i].EndDate != nil {
				if !now.Before(*events[i].StartDate) && !now.After(*events[i].EndDate) {
					targetEvent = &events[i]
					break
				}
			}
		}
	}
	if targetEvent == nil && len(events) > 0 {
		targetEvent = &events[0]
	}

	loc, err := time.LoadLocation(s.conf.Timezone)
	if err != nil || loc == nil {
		loc = time.Local
	}

	var startDay, endDay time.Time
	if targetEvent != nil && targetEvent.StartDate != nil && targetEvent.EndDate != nil {
		sy, sm, sd := targetEvent.StartDate.In(loc).Date()
		startDay = time.Date(sy, sm, sd, 0, 0, 0, 0, loc)
		ey, em, ed := targetEvent.EndDate.In(loc).Date()
		endDay = time.Date(ey, em, ed, 0, 0, 0, 0, loc)
	} else {
		y, m, d := time.Now().In(loc).Date()
		startDay = time.Date(y, m, d, 0, 0, 0, 0, loc)
		endDay = startDay.AddDate(0, 0, 3)
		if targetEvent == nil {
			targetEvent = &storage.TripEvent{
				Code:        "retreat",
				Name:        "Trip Retreat",
				StartDate:   &startDay,
				EndDate:     &endDay,
				Enabled:     true,
			}
		}
	}

	// Guard against excessive range
	if endDay.Sub(startDay) > 14*24*time.Hour {
		endDay = startDay.AddDate(0, 0, 14)
	}

	defs, err := s.storage.GetActiveShiftDefinitions()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active shift definitions: %w", err)
	}

	allUsers, err := s.storage.GetUsers()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch users: %w", err)
	}

	regMap, _ := s.storage.GetUserRegistrationsMap(targetEvent.Code)
	normStats, _ := s.storage.GetTotalNormalizedChoreStats()
	presenceCounts, _ := s.storage.GetUsersPresenceCounts()
	scheduledTasks, _ := s.storage.GetScheduledTasks()
	unfinishedChores, _ := s.storage.GetUnfinishedChores()

	userMap := make(map[string]storage.User, len(allUsers))
	for _, u := range allUsers {
		userMap[u.DiscordId] = u
	}

	// Working copy of simulated workloads: start with normalized stats
	simulatedWorkloads := make(map[string]float64, len(allUsers))
	for u, st := range normStats {
		simulatedWorkloads[u] = st.TotalMin
	}

	now := time.Now().In(loc)
	nowY, nowM, nowD := now.Date()
	startOfToday := time.Date(nowY, nowM, nowD, 0, 0, 0, 0, loc)
	todayStr := startOfToday.Format("2006-01-02")

	days := make([]CalendarDaySchedule, 0)
	curDay := startDay

	for !curDay.After(endDay) {
		dateStr := curDay.Format("2006-01-02")
		isToday := (dateStr == todayStr)
		isPast := curDay.Before(startOfToday)

		// Eligible attendees for this day based on registrations / presence
		dayAttendees := make([]CalendarAttendee, 0)
		for _, u := range allUsers {
			if IsUserEligibleForDate(u, curDay, regMap) {
				reg := regMap[u.DiscordId]
				dayAttendees = append(dayAttendees, CalendarAttendee{
					DiscordID:     u.DiscordId,
					Name:          u.Name,
					Handle:        u.Handle,
					ArrivalDate:   reg.ArrivalDate,
					DepartureDate: reg.DepartureDate,
					IsPresent:     u.IsPresent,
				})
			}
		}

		// Scheduled chores for this day
		dayChores := make([]ScheduledChoreItem, 0)
		dayStart := time.Date(curDay.Year(), curDay.Month(), curDay.Day(), 0, 0, 0, 0, loc)
		dayEnd := time.Date(curDay.Year(), curDay.Month(), curDay.Day(), 23, 59, 59, 0, loc)

		for _, task := range scheduledTasks {
			if !task.Enabled {
				continue
			}
			schedule, err := cronParser.Parse(task.CronExpr)
			if err == nil {
				next := schedule.Next(dayStart.Add(-1 * time.Second))
				if next.Before(dayEnd) || next.Equal(dayStart) {
					dayChores = append(dayChores, ScheduledChoreItem{
						Name:             task.Name,
						Description:      task.Description,
						EstimatedTimeMin: task.EstimatedTimeMin,
						NecessaryWorkers: task.NecessaryWorkers,
						CronExpr:         task.CronExpr,
						Capabilities:     task.GetCapabilities(),
					})
				}
			}
		}

		for _, ch := range unfinishedChores {
			if ch.Deadline != nil {
				dy, dm, dd := ch.Deadline.In(loc).Date()
				if dy == curDay.Year() && dm == curDay.Month() && dd == curDay.Day() {
					dayChores = append(dayChores, ScheduledChoreItem{
						Name:             ch.Name,
						EstimatedTimeMin: ch.EstimatedTimeMin,
						NecessaryWorkers: ch.NecessaryWorkers,
						Capabilities:     ch.GetCapabilities(),
					})
				}
			}
		}

		// Shifts for this day
		existingShifts, _ := s.storage.GetShifts(dateStr)
		isFrozen := false
		for _, es := range existingShifts {
			if es.Frozen {
				isFrozen = true
				break
			}
		}

		dayShifts := make([]PredictedShift, 0)

		if isPast || isFrozen {
			// Real shifts from storage
			for _, sh := range existingShifts {
				var assignments []PredictedAssignment
				for _, a := range sh.Assignments {
					name := a.UserId
					handle := a.UserId
					if u, ok := userMap[a.UserId]; ok {
						name = u.Name
						handle = u.Handle
					}
					assignments = append(assignments, PredictedAssignment{
						UserID:             a.UserId,
						Name:               name,
						Handle:             handle,
						StandbyCreditedMin: a.StandbyCreditedMin,
						Status:             a.Status,
						Predicted:          false,
					})
					simulatedWorkloads[a.UserId] += a.StandbyCreditedMin
				}

				status := "frozen"
				if isPast || sh.Status == "completed" {
					status = "completed"
				}

				dayShifts = append(dayShifts, PredictedShift{
					ID:           sh.ID,
					DefinitionID: sh.DefinitionID,
					Name:         sh.Name,
					Category:     sh.Category,
					DocURL:       sh.DocURL,
					Date:         sh.Date,
					StartTime:    sh.StartTime,
					EndTime:      sh.EndTime,
					WorkersCount: sh.WorkersCount,
					StandbyRatio: sh.StandbyRatio,
					Frozen:       sh.Frozen,
					Predicted:    false,
					Status:       status,
					Assignments:  assignments,
				})
			}
		} else {
			// Generate virtual shift slots from active definitions
			var virtualShifts []storage.Shift
			for _, def := range defs {
				startTime, err1 := parseTimeOfDay(curDay, def.ActiveHoursStart, loc)
				endTime, err2 := parseTimeOfDay(curDay, def.ActiveHoursEnd, loc)
				if err1 != nil || err2 != nil || !endTime.After(startTime) {
					continue
				}

				shiftLen := time.Duration(def.ShiftLengthMin) * time.Minute
				if shiftLen <= 0 {
					shiftLen = 120 * time.Minute
				}

				slotStart := startTime
				for slotStart.Add(shiftLen).Before(endTime) || slotStart.Add(shiftLen).Equal(endTime) {
					slotEnd := slotStart.Add(shiftLen)
					ratio := def.StandbyRatio
					if ratio <= 0 {
						ratio = s.conf.DefaultStandbyRatio
					}
					virtualShifts = append(virtualShifts, storage.Shift{
						ID:           uint(len(virtualShifts) + 1),
						DefinitionID: def.ID,
						Definition:   def,
						Name:         def.Name,
						Category:     def.Category,
						DocURL:       def.DocURL,
						Date:         dateStr,
						StartTime:    slotStart,
						EndTime:      slotEnd,
						WorkersCount: def.WorkersCount,
						StandbyRatio: ratio,
						Frozen:       false,
						Status:       "predicted",
					})
					slotStart = slotEnd
				}
			}

			if len(virtualShifts) > 0 && len(dayAttendees) > 0 {
				input := SolverInput{
					Users:               allUsers,
					Shifts:              virtualShifts,
					NormalizedWorkloads: simulatedWorkloads,
					PresentTicks:        presenceCounts,
					ExistingDayCounts:   make(map[string]int),
					MaxShiftsPerDay:     s.conf.MaxShiftsPerPersonPerDay,
					TargetDate:          curDay,
					Registrations:       regMap,
				}
				solution := SolveOptimizedSchedule(input)

				for _, vs := range virtualShifts {
					assignedIDs := solution[vs.ID]
					var assignments []PredictedAssignment
					for _, uid := range assignedIDs {
						name := uid
						handle := uid
						if u, ok := userMap[uid]; ok {
							name = u.Name
							handle = u.Handle
						}
						standbyMin := vs.StandbyCreditedMin()
						assignments = append(assignments, PredictedAssignment{
							UserID:             uid,
							Name:               name,
							Handle:             handle,
							StandbyCreditedMin: standbyMin,
							Status:             "predicted",
							Predicted:          true,
						})
						simulatedWorkloads[uid] += standbyMin
					}

					dayShifts = append(dayShifts, PredictedShift{
						DefinitionID: vs.DefinitionID,
						Name:         vs.Name,
						Category:     vs.Category,
						DocURL:       vs.DocURL,
						Date:         vs.Date,
						StartTime:    vs.StartTime,
						EndTime:      vs.EndTime,
						WorkersCount: vs.WorkersCount,
						StandbyRatio: vs.StandbyRatio,
						Frozen:       false,
						Predicted:    true,
						Status:       "predicted",
						Assignments:  assignments,
					})
				}
			}
		}

		dayStatus := "idle"
		if isPast {
			dayStatus = "past"
		} else if isFrozen {
			dayStatus = "frozen"
		} else if len(dayShifts) > 0 {
			dayStatus = "predicted"
		}

		days = append(days, CalendarDaySchedule{
			Date:            dateStr,
			DayName:         curDay.Format("Monday"),
			FormattedDate:   curDay.Format("Mon, Jan 02"),
			IsToday:         isToday,
			IsPast:          isPast,
			Status:          dayStatus,
			AttendeesCount:  len(dayAttendees),
			Attendees:       dayAttendees,
			Shifts:          dayShifts,
			ScheduledChores: dayChores,
		})

		curDay = curDay.AddDate(0, 0, 1)
	}

	result := &EventSchedulePrediction{
		EventCode: targetEvent.Code,
		EventName: targetEvent.Name,
		StartDate: startDay,
		EndDate:   endDay,
		Events:    events,
		Days:      days,
	}

	return result, nil
}
