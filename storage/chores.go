package storage

import "gorm.io/gorm/clause"

func (s *Storage) SaveChore(chore Chore) (Chore, error) {
	r := s.db.Save(&chore)
	return chore, r.Error
}

func (s *Storage) GetChore(Id uint) (Chore, error) {
	var chore Chore
	r := s.db.First(&chore, Id)
	if r.Error != nil {
		chore.GetCapabilities()
	}
	return chore, r.Error
}

func (s *Storage) GetChores() ([]Chore, error) {
	var chores []Chore
	r := s.db.Find(&chores)
	if r.Error != nil {
		for i := range chores {
			chores[i].GetCapabilities()
		}
	}
	return chores, r.Error
}

func (s *Storage) GetCompletedChores() ([]Chore, error) {
	var chores []Chore
	r := s.db.Where("completed IS NOT NULL").Order("chores.created DESC").Find(&chores)
	if r.Error != nil {
		for i := range chores {
			chores[i].GetCapabilities()
		}
	}
	return chores, r.Error
}

func (s *Storage) GetUnfinishedChores() ([]Chore, error) {
	var chores []Chore
	r := s.db.Where("completed IS NULL and cancelled IS NULL").Order("created DESC").Find(&chores)
	if r.Error != nil {
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
	r := s.db.Save(&wl)
	return wl, r.Error
}

func (s *Storage) GetWorkLogs() ([]WorkLog, error) {
	var worklogs []WorkLog
	r := s.db.Preload(clause.Associations).Find(&worklogs)
	return worklogs, r.Error
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
