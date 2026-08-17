package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Storage) GetLastSuccessfulLLMSummaryLog() (*LLMSummaryLog, error) {
	var log LLMSummaryLog
	r := s.db.Where("status = ?", "success").Order("run_at DESC").First(&log)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &log, nil
}

func (s *Storage) SaveLLMSummaryLog(log *LLMSummaryLog) error {
	r := s.db.Save(log)
	return r.Error
}

func (s *Storage) GetTasksActivitySince(since time.Time) (*TasksActivity, error) {
	activity := &TasksActivity{}

	// 1. Created chores since the cutoff
	var createdChores []Chore
	r := s.db.Where("created >= ?", since).Order("created ASC").Find(&createdChores)
	if r.Error != nil {
		return nil, r.Error
	}
	for i := range createdChores {
		createdChores[i].GetCapabilities()
	}
	activity.CreatedChores = createdChores

	// 2. Completed chores since the cutoff
	var completedChores []Chore
	r = s.db.Where("completed IS NOT NULL AND completed >= ?", since).Order("completed ASC").Find(&completedChores)
	if r.Error != nil {
		return nil, r.Error
	}
	for i := range completedChores {
		completedChores[i].GetCapabilities()
	}
	activity.CompletedChores = completedChores

	// 3. Cancelled chores since the cutoff
	var cancelledChores []Chore
	r = s.db.Where("cancelled IS NOT NULL AND cancelled >= ?", since).Order("cancelled ASC").Find(&cancelledChores)
	if r.Error != nil {
		return nil, r.Error
	}
	for i := range cancelledChores {
		cancelledChores[i].GetCapabilities()
	}
	activity.CancelledChores = cancelledChores

	// 4. Assignments created or updated since the cutoff
	var assignments []ChoreAssignment
	r = s.db.Preload(clause.Associations).
		Where("created >= ? OR acked >= ? OR refused >= ? OR timeouted >= ?", since, since, since, since).
		Find(&assignments)
	if r.Error != nil {
		return nil, r.Error
	}
	activity.UpdatedAssignments = assignments

	// 5. Currently active/unfinished chores
	var activeChores []Chore
	r = s.db.Where("completed IS NULL AND cancelled IS NULL").Order("created ASC").Find(&activeChores)
	if r.Error != nil {
		return nil, r.Error
	}
	for i := range activeChores {
		activeChores[i].GetCapabilities()
	}
	activity.ActiveChores = activeChores

	// 6. Work logs associated with completed chores or logged since the cutoff
	var workLogs []WorkLog
	r = s.db.Preload(clause.Associations).Find(&workLogs)
	if r.Error != nil {
		return nil, r.Error
	}
	// Filter worklogs for chores that were completed since `since`
	completedChoreIDs := make(map[uint]bool)
	for _, c := range completedChores {
		completedChoreIDs[c.ID] = true
	}
	var relevantWorkLogs []WorkLog
	for _, wl := range workLogs {
		if completedChoreIDs[wl.ChoreId] {
			relevantWorkLogs = append(relevantWorkLogs, wl)
		}
	}
	activity.WorkLogs = relevantWorkLogs

	return activity, nil
}

func (s *Storage) GetUserHandlesMap() (map[string]string, error) {
	handles := make(map[string]string)

	if s.discord != nil && s.conf.DiscordGuildId != "" && s.conf.DiscordGuildId != "???" {
		members, err := s.discord.GuildMembers(s.conf.DiscordGuildId, "", 1000)
		if err == nil {
			for _, m := range members {
				if m.User != nil {
					displayName := m.User.Username
					if m.Nick != "" {
						displayName = m.Nick
					}
					handles[m.User.ID] = displayName
				}
			}
		}
	}

	return handles, nil
}
