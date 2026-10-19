package web

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

func round1(val float64) float64 {
	return math.Round(val*10) / 10
}

const (
	SpicyPepper       = "🌶️"
	UrgentDeadlineMin = 60
)

func SpicinessOf(name string) int {
	c := strings.Count(name, "🌶")
	if c > 3 {
		return 3
	}
	return c
}

func SizeFor(minutes uint) string {
	if minutes <= 10 {
		return "small"
	}
	if minutes <= 30 {
		return "medium"
	}
	return "large"
}

func (w *Web) BuildPersonDirectory() map[string]UserInfo {
	dir := make(map[string]UserInfo)

	// 1. Fetch all known guild users from Discord cache and database
	guildUsers, err := w.storage.GetAllGuildUsers()
	if err == nil {
		for _, u := range guildUsers {
			name := u.Name
			if name == "" {
				name = u.Handle
			}
			if name == "" {
				name = u.DiscordId
			}
			dir[u.DiscordId] = UserInfo{
				DiscordId:    u.DiscordId,
				Name:         name,
				Handle:       u.Handle,
				Capabilities: u.Capabilities,
				IsPresent:    u.IsPresent,
			}
		}
	}

	// 2. Augment with stored user profiles if available
	profiles, err := w.storage.GetAllProfiles()
	if err == nil {
		for _, p := range profiles {
			entry, ok := dir[p.DiscordId]
			if !ok {
				entry = UserInfo{
					DiscordId: p.DiscordId,
				}
			}
			if p.Name != "" {
				entry.Name = p.Name
			}
			if p.DiscordHandle != "" {
				entry.Handle = p.DiscordHandle
			}
			if entry.Name == "" {
				entry.Name = entry.Handle
			}
			if entry.Name == "" {
				entry.Name = p.DiscordId
			}
			dir[p.DiscordId] = entry
		}
	}

	// 3. Mark currently present users from Discord role (chores::present)
	users, err := w.storage.GetPresentUsers()
	if err == nil {
		for _, u := range users {
			entry, ok := dir[u.DiscordId]
			if !ok {
				name := u.Name
				if name == "" {
					name = u.Handle
				}
				entry = UserInfo{
					DiscordId: u.DiscordId,
					Name:      name,
					Handle:    u.Handle,
				}
			}
			entry.Capabilities = u.Capabilities
			entry.IsPresent = true
			dir[u.DiscordId] = entry
		}
	}

	return dir
}

func (w *Web) BuildChoreView(chore storage.Chore, worklogs []storage.WorkLog, assignments []storage.ChoreAssignment, delayed *storage.DelayedTask, dir map[string]UserInfo) ChoreView {
	caps := chore.GetCapabilities()
	est := chore.EstimatedTimeMin
	size := SizeFor(est)
	spiciness := SpicinessOf(chore.Name)
	urgent := spiciness > 0

	var minutesToDeadline *int
	now := time.Now()
	if chore.Deadline != nil {
		diff := int(chore.Deadline.Sub(now).Minutes())
		minutesToDeadline = &diff
		if diff <= UrgentDeadlineMin {
			urgent = true
		}
	}

	isDelayed := false
	var minutesToPublish *int
	var publishAt *time.Time
	if delayed != nil && delayed.ExecutedAt == nil {
		publishAt = &delayed.PublishAt
		if delayed.PublishAt.After(now) {
			isDelayed = true
			diff := int(delayed.PublishAt.Sub(now).Minutes())
			minutesToPublish = &diff
		}
	}

	// Claimers from acked assignments
	var claimers []ClaimerView
	for _, a := range assignments {
		if a.Acked != nil {
			name := a.UserId
			if u, ok := dir[a.UserId]; ok && u.Name != "" {
				name = u.Name
			} else {
				resolved := w.storage.ResolveUserName(a.UserId)
				if resolved != "" {
					name = resolved
				}
			}
			claimers = append(claimers, ClaimerView{
				DiscordId: a.UserId,
				Name:      name,
			})
		}
	}

	// Worklogs
	var wlViews []WorkLogView
	var workedMinTotal uint
	for _, wl := range worklogs {
		workedMinTotal += wl.TimeSpentMin
		wlViews = append(wlViews, WorkLogView{
			ChoreId:      wl.ChoreId,
			UserId:       wl.UserId,
			TimeSpentMin: wl.TimeSpentMin,
			SelfReported: wl.SelfReported,
		})
	}

	creatorName := chore.CreatorId
	if u, ok := dir[chore.CreatorId]; ok && u.Name != "" {
		creatorName = u.Name
	} else if chore.CreatorId != "" && chore.CreatorId != "API" {
		resolved := w.storage.ResolveUserName(chore.CreatorId)
		if resolved != "" {
			creatorName = resolved
		}
	}

	workers := chore.NecessaryWorkers
	if workers == 0 {
		workers = 1
	}

	active := chore.Completed == nil && chore.Cancelled == nil

	return ChoreView{
		ID:                    chore.ID,
		Name:                  chore.Name,
		Event:                 chore.Event,
		NecessaryWorkers:      workers,
		EstimatedTimeMin:      est,
		AssignmentTimeoutMin:  chore.AssignmentTimeoutMin,
		NecessaryCapabilities: caps,
		Deadline:              chore.Deadline,
		MinutesToDeadline:     minutesToDeadline,
		Completed:             chore.Completed,
		Cancelled:             chore.Cancelled,
		Created:               chore.Created,
		DelayMin:              chore.DelayMin,
		PublishAt:             publishAt,
		IsDelayed:             isDelayed,
		MinutesToPublish:      minutesToPublish,
		SelfReported:          chore.SelfReported,
		CreatorId:             chore.CreatorId,
		CreatorName:           creatorName,
		Worklogs:              wlViews,
		WorkedMinTotal:        workedMinTotal,
		Size:                  size,
		Urgent:                urgent,
		Spiciness:             spiciness,
		TemplateKey:           chore.TemplateKey,
		Claimers:              claimers,
		ClaimedCount:          len(claimers),
		FullyClaimed:          len(claimers) >= int(workers),
		Active:                active,
		TotalTimeMin:          workedMinTotal,
	}
}

func (w *Web) ListChoreViews(activeOnly bool) ([]ChoreView, error) {
	return w.ListChoreViewsForEvent(activeOnly, "")
}

func (w *Web) ListChoreViewsForEvent(activeOnly bool, event string) ([]ChoreView, error) {
	chores, err := w.storage.GetChoresForEvent(activeOnly, event)
	if err != nil {
		return nil, err
	}

	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForEvent(event)
	wlByChore := make(map[uint][]storage.WorkLog)
	for _, wl := range worklogs {
		wlByChore[wl.ChoreId] = append(wlByChore[wl.ChoreId], wl)
	}

	assignments, _ := w.storage.GetChoresAssignments()
	asByChore := make(map[uint][]storage.ChoreAssignment)
	for _, a := range assignments {
		asByChore[a.ChoreId] = append(asByChore[a.ChoreId], a)
	}

	delayedTasks, _ := w.storage.GetDelayedTasks()
	delayedByChore := make(map[uint]*storage.DelayedTask)
	for i := range delayedTasks {
		if delayedTasks[i].ExecutedAt == nil {
			delayedByChore[delayedTasks[i].ChoreID] = &delayedTasks[i]
		}
	}

	var views []ChoreView
	for _, c := range chores {
		view := w.BuildChoreView(c, wlByChore[c.ID], asByChore[c.ID], delayedByChore[c.ID], dir)
		if activeOnly && !view.Active {
			continue
		}
		views = append(views, view)
	}

	// Sort: urgent first, then soonest deadline, then largest time
	sort.Slice(views, func(i, j int) bool {
		if views[i].Urgent != views[j].Urgent {
			return views[i].Urgent
		}
		m1 := 10000000
		if views[i].MinutesToDeadline != nil {
			m1 = *views[i].MinutesToDeadline
		}
		m2 := 10000000
		if views[j].MinutesToDeadline != nil {
			m2 = *views[j].MinutesToDeadline
		}
		if m1 != m2 {
			return m1 < m2
		}
		return views[i].EstimatedTimeMin > views[j].EstimatedTimeMin
	})

	return views, nil
}

func (w *Web) SuggestionsFor(chore storage.Chore) SuggestionsResult {
	dir := w.BuildPersonDirectory()
	stats, _ := w.storage.GetUserStats()
	assignments, _ := w.storage.GetChoreAssignments(chore.ID)
	claimedSet := make(map[string]bool)
	for _, a := range assignments {
		if a.Acked != nil {
			claimedSet[a.UserId] = true
		}
	}

	reqSkills := chore.GetCapabilities()

	type candidate struct {
		user     UserInfo
		score    int
		hasSkill bool
		claimed  bool
	}

	var candidates []candidate
	for id, u := range dir {
		hasSkill := true
		if len(reqSkills) > 0 {
			userSkills := make(map[string]bool)
			for _, s := range u.Capabilities {
				userSkills[s] = true
			}
			for _, req := range reqSkills {
				if !userSkills[req] {
					hasSkill = false
					break
				}
			}
		}

		userStat := stats[id]
		score := int(userStat.TotalMin)

		candidates = append(candidates, candidate{
			user:     u,
			score:    score,
			hasSkill: hasSkill,
			claimed:  claimedSet[id],
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		// Claimed at the bottom
		if candidates[i].claimed != candidates[j].claimed {
			return !candidates[i].claimed
		}
		// People with skills first
		if candidates[i].hasSkill != candidates[j].hasSkill {
			return candidates[i].hasSkill
		}
		// Lowest workload score first
		return candidates[i].score < candidates[j].score
	})

	var ranked []SuggestedPerson
	var top []string
	for _, c := range candidates {
		ranked = append(ranked, SuggestedPerson{
			DiscordId: c.user.DiscordId,
			Name:      c.user.Name,
			HasSkill:  c.hasSkill,
			Claimed:   c.claimed,
			LoadScore: c.score,
		})
		if !c.claimed && c.hasSkill && len(top) < 3 {
			top = append(top, c.user.DiscordId)
		}
	}

	return SuggestionsResult{
		Top:    top,
		Ranked: ranked,
	}
}

func (w *Web) Leaderboard() []LeaderboardRow {
	return w.LeaderboardForEvent("")
}

func (w *Web) LeaderboardForEvent(event string) []LeaderboardRow {
	dir := w.BuildPersonDirectory()
	stats, _ := w.storage.GetUserStatsForEvent(event)
	assignments, _ := w.storage.GetChoresAssignments()

	assignedCounts := make(map[string]int)
	for _, a := range assignments {
		if a.Acked == nil && a.Refused == nil && a.Timeouted == nil {
			assignedCounts[a.UserId]++
		}
	}

	var rows []LeaderboardRow
	for id, s := range stats {
		name := id
		if u, ok := dir[id]; ok && u.Name != "" {
			name = u.Name
		} else {
			resolved := w.storage.ResolveUserName(id)
			if resolved != "" {
				name = resolved
			}
		}
		rows = append(rows, LeaderboardRow{
			DiscordId:     id,
			Name:          name,
			WorkedCount:   int(s.Count),
			WorkedMin:     round1(s.TotalMin),
			AssignedCount: assignedCounts[id],
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].WorkedMin != rows[j].WorkedMin {
			return rows[i].WorkedMin > rows[j].WorkedMin
		}
		if rows[i].WorkedCount != rows[j].WorkedCount {
			return rows[i].WorkedCount > rows[j].WorkedCount
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})

	return rows
}

func (w *Web) UserDetail(discordID string) UserDetail {
	return w.UserDetailForEvent(discordID, "")
}

func (w *Web) UserDetailForEvent(discordID string, event string) UserDetail {
	dir := w.BuildPersonDirectory()
	u := dir[discordID]
	name := discordID
	handle := ""
	if u.Name != "" {
		name = u.Name
		handle = u.Handle
	} else {
		resolved := w.storage.ResolveUserName(discordID)
		if resolved != "" {
			name = resolved
		}
	}

	allChores, _ := w.storage.GetChoresForEvent(false, "all")
	choreMap := make(map[uint]storage.Chore)
	for _, c := range allChores {
		choreMap[c.ID] = c
	}

	worklogs, _ := w.storage.GetWorkLogsForEvent("all")
	wlByChore := make(map[uint][]storage.WorkLog)
	userWorklogByChore := make(map[uint]storage.WorkLog)
	userEventMinutes := make(map[string]uint)
	userEventCounts := make(map[string]int)

	for _, wl := range worklogs {
		wlByChore[wl.ChoreId] = append(wlByChore[wl.ChoreId], wl)
		if wl.UserId == discordID {
			userWorklogByChore[wl.ChoreId] = wl
			c := choreMap[wl.ChoreId]
			ev := wl.Event
			if ev == "" {
				ev = c.Event
			}
			userEventMinutes[ev] += wl.TimeSpentMin
			userEventCounts[ev]++
		}
	}

	assignments, _ := w.storage.GetChoresAssignments()
	asByChore := make(map[uint][]storage.ChoreAssignment)
	for _, a := range assignments {
		asByChore[a.ChoreId] = append(asByChore[a.ChoreId], a)
	}

	var performing []ChoreView
	var performed []ChoreView
	var totalTime uint

	seenChores := make(map[uint]bool)
	for _, a := range assignments {
		if a.UserId == discordID && a.Acked != nil {
			c, ok := choreMap[a.ChoreId]
			if !ok || c.Cancelled != nil || seenChores[c.ID] {
				continue
			}
			seenChores[c.ID] = true

			// Filter by event if requested
			if event != "" && event != "all" && c.Event != event {
				continue
			}

			view := w.BuildChoreView(c, wlByChore[c.ID], asByChore[c.ID], nil, dir)
			if c.Completed != nil {
				userMin := uint(0)
				if wl, ok := userWorklogByChore[c.ID]; ok {
					userMin = wl.TimeSpentMin
				} else if c.NecessaryWorkers > 0 {
					userMin = uint(c.EstimatedTimeMin / c.NecessaryWorkers)
				} else {
					userMin = uint(c.EstimatedTimeMin)
				}
				totalTime += userMin
				performed = append(performed, view)
			} else {
				performing = append(performing, view)
			}
		}
	}

	// Also check worklogs for user (in case of manual work or self-reported chores)
	for _, wl := range worklogs {
		if wl.UserId == discordID {
			c, ok := choreMap[wl.ChoreId]
			if ok && !seenChores[c.ID] && c.Cancelled == nil {
				seenChores[c.ID] = true
				if event != "" && event != "all" && c.Event != event {
					continue
				}
				view := w.BuildChoreView(c, wlByChore[c.ID], asByChore[c.ID], nil, dir)
				if c.Completed != nil {
					totalTime += wl.TimeSpentMin
					performed = append(performed, view)
				} else {
					performing = append(performing, view)
				}
			}
		}
	}

	// Build event history summaries
	events, _ := w.storage.GetTripEvents()
	eventNameMap := make(map[string]string)
	for _, e := range events {
		eventNameMap[e.Code] = e.Name
	}
	if eventNameMap["g::t::6.9"] == "" {
		eventNameMap["g::t::6.9"] = "Garage Trip 6.9 (2025)"
	}
	if eventNameMap["g::t::7.0.0"] == "" {
		eventNameMap["g::t::7.0.0"] = "Garage Trip 7.0 (2026)"
	}

	var eventHistory []UserEventSummary
	for ev, mins := range userEventMinutes {
		name := eventNameMap[ev]
		if name == "" {
			name = ev
		}
		eventHistory = append(eventHistory, UserEventSummary{
			Event:        ev,
			EventName:    name,
			WorkedCount:  userEventCounts[ev],
			TimeSpentMin: mins,
		})
	}
	sort.Slice(eventHistory, func(i, j int) bool {
		return eventHistory[i].Event > eventHistory[j].Event
	})

	return UserDetail{
		DiscordId:    discordID,
		Name:         name,
		Handle:       handle,
		Performing:   performing,
		Performed:    performed,
		TimeSpentMin: totalTime,
		EventHistory: eventHistory,
	}
}

func (w *Web) GetPeoplePoolForEvent(event string) []PersonPoolEntry {
	dir := w.BuildPersonDirectory()
	stats, _ := w.storage.GetAggregatedStatsForEvent(event)

	// Calculate committed minutes for active in-progress chores
	committed := make(map[string]float64)
	chores, _ := w.storage.GetChoresForEvent(false, event)
	assignments, _ := w.storage.GetChoresAssignments()
	ackMap := make(map[uint][]string)
	for _, a := range assignments {
		if a.Acked != nil {
			ackMap[a.ChoreId] = append(ackMap[a.ChoreId], a.UserId)
		}
	}
	for _, c := range chores {
		if c.Completed == nil && c.Cancelled == nil {
			for _, uid := range ackMap[c.ID] {
				committed[uid] += float64(c.EstimatedTimeMin)
			}
		}
	}

	var pool []PersonPoolEntry
	for _, u := range dir {
		s := stats[u.DiscordId]
		workload := s.TotalMin + committed[u.DiscordId]
		pool = append(pool, PersonPoolEntry{
			DiscordId:       u.DiscordId,
			Name:            u.Name,
			Handle:          u.Handle,
			Capabilities:    u.Capabilities,
			WorkloadMin:     round1(workload),
			NormalizedTotal: round1(s.NormalizedTotal),
			PresentTicks:    s.PresentTicks,
		})
	}

	sort.Slice(pool, func(i, j int) bool {
		if pool[i].NormalizedTotal != pool[j].NormalizedTotal {
			return pool[i].NormalizedTotal < pool[j].NormalizedTotal
		}
		return pool[i].WorkloadMin < pool[j].WorkloadMin
	})
	return pool
}

func (w *Web) GetPeoplePool() []PersonPoolEntry {
	return w.GetPeoplePoolForEvent("")
}

