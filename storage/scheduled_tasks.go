package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

func (s *Storage) CreateScheduledTask(task ScheduledTask) (ScheduledTask, error) {
	now := time.Now()
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.NecessaryCapabilities == "" {
		task.NecessaryCapabilities = "[]"
	}
	if task.NecessaryWorkers == 0 {
		task.NecessaryWorkers = 1
	}
	if task.EstimatedTimeMin == 0 {
		task.EstimatedTimeMin = 10
	}
	if task.AssignmentTimeoutMin == 0 {
		task.AssignmentTimeoutMin = 15
	}
	r := s.db.Create(&task)
	return task, r.Error
}

func (s *Storage) GetScheduledTasks() ([]ScheduledTask, error) {
	var list []ScheduledTask
	r := s.db.Order("created_at asc").Find(&list)
	return list, r.Error
}

func (s *Storage) GetScheduledTask(id uint) (*ScheduledTask, error) {
	var task ScheduledTask
	r := s.db.First(&task, id)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &task, nil
}

func (s *Storage) UpdateScheduledTask(task ScheduledTask) error {
	task.UpdatedAt = time.Now()
	return s.db.Model(&ScheduledTask{}).
		Where("id = ?", task.ID).
		Updates(map[string]any{
			"name":                   task.Name,
			"description":            task.Description,
			"cron_expr":              task.CronExpr,
			"necessary_workers":      task.NecessaryWorkers,
			"estimated_time_min":     task.EstimatedTimeMin,
			"assignment_timeout_min": task.AssignmentTimeoutMin,
			"necessary_capabilities": task.NecessaryCapabilities,
			"creator_id":             task.CreatorId,
			"creator_name":           task.CreatorName,
			"enabled":                task.Enabled,
			"template_key":           task.TemplateKey,
			"next_run_at":            task.NextRunAt,
			"updated_at":             task.UpdatedAt,
		}).Error
}

func (s *Storage) DeleteScheduledTask(id uint) error {
	return s.db.Delete(&ScheduledTask{}, id).Error
}

func (s *Storage) UpdateScheduledTaskRun(id uint, lastRun time.Time, nextRun *time.Time) error {
	return s.db.Model(&ScheduledTask{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_run_at": &lastRun,
			"next_run_at": nextRun,
			"updated_at":  time.Now(),
		}).Error
}

func (s *Storage) GetDueScheduledTasks(now time.Time) ([]ScheduledTask, error) {
	var list []ScheduledTask
	r := s.db.Where("enabled = ? AND next_run_at IS NOT NULL AND next_run_at <= ?", true, now).
		Find(&list)
	return list, r.Error
}
