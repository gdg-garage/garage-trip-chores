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
	r := s.db.Where("completed IS NOT NULL").Find(&chores)
	if r.Error != nil {
		for i := range chores {
			chores[i].GetCapabilities()
		}
	}
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
