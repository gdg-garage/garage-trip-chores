package storage

import "gorm.io/gorm/clause"

func (s *Storage) SaveChore(chore Chore) (Chore, error) {
	if chore.Event == "" {
		chore.Event = s.GetActiveEventCode()
	}
	isNew := chore.ID == 0
	r := s.db.Save(&chore)
	if r.Error == nil && s.Events != nil && !chore.Draft {
		eventType := TaskUpdated
		if isNew {
			eventType = TaskCreated
		} else if chore.Completed != nil {
			eventType = TaskDone
		}
		s.Events.Publish(Event{
			Type:  eventType,
			Chore: &chore,
		})
	}
	return chore, r.Error
}

func (s *Storage) GetChore(Id uint) (Chore, error) {
	var chore Chore
	r := s.db.First(&chore, Id)
	if r.Error == nil {
		chore.GetCapabilities()
	}
	return chore, r.Error
}

func (s *Storage) DeleteChore(id uint) error {
	return s.db.Delete(&Chore{}, id).Error
}

func (s *Storage) GetChoresForEvent(activeOnly bool, event string) ([]Chore, error) {
	var chores []Chore
	query := s.db
	if activeOnly {
		query = query.Where("completed IS NULL AND cancelled IS NULL")
	}
	query = query.Where("draft = ? OR draft IS NULL", false)
	if event != "" && event != "all" {
		query = query.Where("event = ?", event)
	}
	r := query.Find(&chores)
	if r.Error == nil {
		for i := range chores {
			chores[i].GetCapabilities()
		}
	}
	return chores, r.Error
}

func (s *Storage) GetChores() ([]Chore, error) {
	return s.GetChoresForEvent(false, "")
}

func (s *Storage) GetCompletedChores() ([]Chore, error) {
	var chores []Chore
	r := s.db.Where("completed IS NOT NULL AND (draft = ? OR draft IS NULL)", false).Order("chores.created DESC").Find(&chores)
	if r.Error == nil {
		for i := range chores {
			chores[i].GetCapabilities()
		}
	}
	return chores, r.Error
}

func (s *Storage) GetUnfinishedChores() ([]Chore, error) {
	var chores []Chore
	r := s.db.Where("completed IS NULL and cancelled IS NULL and (draft = ? OR draft IS NULL)", false).Order("created DESC").Find(&chores)
	if r.Error == nil {
		for i := range chores {
			chores[i].GetCapabilities()
		}
	}
	return chores, r.Error
}

func (s *Storage) GetAssignedChoresForUser(userId string) ([]Chore, error) {
	var chores []Chore
	r := s.db.Joins("JOIN chore_assignments ON chore_assignments.chore_id = chores.id").
		Where("chore_assignments.user_id = ? AND chore_assignments.acked IS NULL AND chore_assignments.refused IS NULL AND chore_assignments.timeouted IS NULL and chores.completed IS NULL and chores.cancelled IS NULL", userId).
		Order("chores.created DESC").
		Find(&chores)
	return chores, r.Error
}

func (s *Storage) GetAckedChoresForUser(userId string) ([]Chore, error) {
	var chores []Chore
	r := s.db.Joins("JOIN chore_assignments ON chore_assignments.chore_id = chores.id").
		Where("chore_assignments.user_id = ? AND chore_assignments.acked IS NOT NULL and chores.completed IS NULL and chores.cancelled IS NULL", userId).
		Order("chores.created DESC").
		Find(&chores)
	return chores, r.Error
}

func (s *Storage) SaveWorkLog(wl WorkLog) (WorkLog, error) {
	if wl.Event == "" {
		if wl.Chore.Event != "" {
			wl.Event = wl.Chore.Event
		} else if wl.ChoreId != 0 {
			var ch Chore
			if err := s.db.Select("event").First(&ch, wl.ChoreId).Error; err == nil && ch.Event != "" {
				wl.Event = ch.Event
			}
		}
		if wl.Event == "" {
			wl.Event = s.GetActiveEventCode()
		}
	}
	r := s.db.Save(&wl)
	return wl, r.Error
}

func (s *Storage) GetWorkLogsForEvent(event string) ([]WorkLog, error) {
	var worklogs []WorkLog
	query := s.db.Preload(clause.Associations)
	if event != "" && event != "all" {
		query = query.Where("event = ?", event)
	}
	r := query.Find(&worklogs)
	return worklogs, r.Error
}

func (s *Storage) GetWorkLogs() ([]WorkLog, error) {
	return s.GetWorkLogsForEvent("")
}

func (s *Storage) GetWorkLogsForChore(choreId uint) ([]WorkLog, error) {
	var worklogs []WorkLog
	r := s.db.Preload(clause.Associations).Where("chore_id = ?", choreId).Find(&worklogs)
	return worklogs, r.Error
}

func (s *Storage) GetWorkLogForChoreAndUser(choreId uint, userId string) (WorkLog, error) {
	var worklog WorkLog
	r := s.db.Preload(clause.Associations).Where("chore_id = ? AND user_id = ?", choreId, userId).First(&worklog)
	return worklog, r.Error
}
