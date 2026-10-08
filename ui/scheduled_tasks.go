package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

func getTimeLocation() *time.Location {
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		return time.FixedZone("CET", 1*3600)
	}
	return loc
}

func ParseCronNext(cronExpr string, from time.Time, loc *time.Location) (*time.Time, error) {
	if loc == nil {
		loc = getTimeLocation()
	}
	schedule, err := cronParser.Parse(cronExpr)
	if err != nil {
		return nil, err
	}
	next := schedule.Next(from.In(loc))
	return &next, nil
}


func (ui *Ui) ExecuteScheduledTask(taskID uint) (*storage.Chore, error) {
	task, err := ui.storage.GetScheduledTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scheduled task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("scheduled task with id %d not found", taskID)
	}

	chore := storage.Chore{
		Name:                  task.Name,
		NecessaryWorkers:      task.NecessaryWorkers,
		EstimatedTimeMin:      task.EstimatedTimeMin,
		AssignmentTimeoutMin:  task.AssignmentTimeoutMin,
		NecessaryCapabilities: task.NecessaryCapabilities,
		AssigneeId:            task.AssigneeId,
		CreatorId:             task.CreatorId, // CRITICAL: keeps the creator of the task!
		TemplateKey:           task.TemplateKey,
		Draft:                 false,
		Created:               time.Now(),
	}

	publishedChore, _, err := ui.PublishChore(chore)
	if err != nil {
		ui.logger.Error("failed to publish scheduled chore", "task_id", task.ID, "error", err)
		return nil, err
	}

	now := time.Now()
	nextRun, err := ParseCronNext(task.CronExpr, now, getTimeLocation())
	if err != nil {
		ui.logger.Warn("failed to calculate next run after execution", "task_id", task.ID, "cron", task.CronExpr, "error", err)
	}

	if err := ui.storage.UpdateScheduledTaskRun(task.ID, now, nextRun); err != nil {
		ui.logger.Error("failed to update scheduled task run timestamps", "task_id", task.ID, "error", err)
	}

	ui.logger.Info("Executed scheduled task", "task_id", task.ID, "name", task.Name, "creator_id", task.CreatorId, "chore_id", publishedChore.ID)
	return &publishedChore, nil
}

func (ui *Ui) ProcessDueScheduledTasks() {
	now := time.Now()
	loc := getTimeLocation()

	// Ensure all enabled tasks have a valid NextRunAt
	tasks, err := ui.storage.GetScheduledTasks()
	if err == nil {
		for _, t := range tasks {
			if t.Enabled && (t.NextRunAt == nil || t.NextRunAt.Before(now.Add(-24*time.Hour))) {
				next, err := ParseCronNext(t.CronExpr, now, loc)
				if err == nil && next != nil {
					t.NextRunAt = next
					_ = ui.storage.UpdateScheduledTask(t)
				}
			}
		}
	}

	dueTasks, err := ui.storage.GetDueScheduledTasks(now)
	if err != nil {
		ui.logger.Error("failed to query due scheduled tasks", "error", err)
		return
	}

	for _, dt := range dueTasks {
		ui.logger.Info("Triggering due scheduled task", "task_id", dt.ID, "name", dt.Name, "cron", dt.CronExpr)
		_, err := ui.ExecuteScheduledTask(dt.ID)
		if err != nil {
			ui.logger.Error("failed executing scheduled task", "task_id", dt.ID, "error", err)
		}
	}
}

func (ui *Ui) RunScheduledTaskScheduler(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ui.logger.Info("Scheduled task scheduler started")

	// Run once immediately on start
	ui.ProcessDueScheduledTasks()

	for {
		select {
		case <-ctx.Done():
			ui.logger.Debug("Scheduled task scheduler stopped: context cancelled")
			return
		case <-ticker.C:
			ui.ProcessDueScheduledTasks()
		}
	}
}
