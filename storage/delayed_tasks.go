package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Storage) CreateDelayedTask(choreID uint, publishAt time.Time, delayMin uint) (DelayedTask, error) {
	dt := DelayedTask{
		ChoreID:   choreID,
		PublishAt: publishAt,
		DelayMin:  delayMin,
		CreatedAt: time.Now(),
	}
	r := s.db.Create(&dt)
	return dt, r.Error
}

func (s *Storage) GetPendingDelayedTasks(now time.Time) ([]DelayedTask, error) {
	var tasks []DelayedTask
	r := s.db.Preload(clause.Associations).
		Where("executed_at IS NULL AND publish_at <= ?", now).
		Order("publish_at ASC").
		Find(&tasks)
	return tasks, r.Error
}

func (s *Storage) MarkDelayedTaskExecuted(id uint) error {
	now := time.Now()
	r := s.db.Model(&DelayedTask{}).
		Where("id = ?", id).
		Update("executed_at", &now)
	return r.Error
}

func (s *Storage) CancelDelayedTaskByChoreId(choreID uint) error {
	r := s.db.Where("chore_id = ? AND executed_at IS NULL", choreID).
		Delete(&DelayedTask{})
	return r.Error
}

func (s *Storage) GetDelayedTaskForChore(choreID uint) (*DelayedTask, error) {
	var dt DelayedTask
	r := s.db.Preload(clause.Associations).
		Where("chore_id = ? AND executed_at IS NULL", choreID).
		First(&dt)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &dt, nil
}

func (s *Storage) GetDelayedTasks() ([]DelayedTask, error) {
	var tasks []DelayedTask
	r := s.db.Preload(clause.Associations).
		Order("created_at DESC").
		Find(&tasks)
	return tasks, r.Error
}
