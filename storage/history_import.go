package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// MigrateAndImportHistory backfills event tags on legacy rows and imports 2025 historical data if available.
func (s *Storage) MigrateAndImportHistory(db2025Path string) error {
	// 1. Backfill legacy GT7 data where event is unset
	gt7Code := "g::t::7.0.0"
	s.db.Exec("UPDATE chores SET event = ? WHERE event = '' OR event IS NULL", gt7Code)
	s.db.Exec("UPDATE work_logs SET event = ? WHERE event = '' OR event IS NULL", gt7Code)
	s.db.Exec("UPDATE presence_logs SET event = ? WHERE event = '' OR event IS NULL", gt7Code)
	s.db.Exec("UPDATE shifts SET event = ? WHERE event = '' OR event IS NULL", gt7Code)
	s.db.Exec("UPDATE llm_summary_logs SET event = ? WHERE event = '' OR event IS NULL", gt7Code)

	// 2. Locate 2025 historical database
	gt2025Path := db2025Path
	if gt2025Path == "" {
		candidates := []string{
			"data-prod/gt6.9-2025.sqlite",
			"data/gt6.9-2025.sqlite",
			"/data-prod/gt6.9-2025.sqlite",
			"/root/chores-data/gt6.9-2025.sqlite",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				gt2025Path = c
				break
			}
		}
	}

	if gt2025Path == "" {
		return nil
	}

	absPath, err := filepath.Abs(gt2025Path)
	if err != nil {
		return fmt.Errorf("failed to resolve path for 2025 sqlite: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil
	}

	// 3. Check if already imported
	var existingCount int64
	s.db.Model(&Chore{}).Where("event = ?", "g::t::6.9").Count(&existingCount)
	if existingCount > 0 {
		s.logger.Info("Historical 2025 data already imported, skipping", "count", existingCount)
		return nil
	}

	s.logger.Info("Importing historical 2025 data from SQLite", "path", absPath)

	// 4. Attach database and import records in a transaction
	// 4. Attach database and import records in a transaction
	if err := s.db.Exec("ATTACH DATABASE ? AS db2025", absPath).Error; err != nil {
		return fmt.Errorf("failed to attach 2025 db: %w", err)
	}
	defer func() {
		_ = s.db.Exec("DETACH DATABASE db2025")
	}()

	tx := s.db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Chores offset by +10000
	choreQuery := `
		INSERT INTO chores (
			id, name, necessary_capabilities, necessary_workers,
			estimated_time_min, assignment_timeout_min, creator_id, message_id,
			created, completed, cancelled, deadline, after_deadline_reminded,
			draft, self_reported, event
		)
		SELECT
			id + 10000, name, necessary_capabilities, necessary_workers,
			estimated_time_min, assignment_timeout_min, creator_id, message_id,
			created, completed, cancelled, deadline, after_deadline_reminded,
			0, 0, 'g::t::6.9'
		FROM db2025.chores
	`
	if err := tx.Exec(choreQuery).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to import 2025 chores: %w", err)
	}

	// Work logs remapped to chore_id + 10000
	workLogQuery := `
		INSERT INTO work_logs (user_id, chore_id, time_spent_min, self_reported, event)
		SELECT user_id, chore_id + 10000, time_spent_min, self_reported, 'g::t::6.9'
		FROM db2025.work_logs
	`
	if err := tx.Exec(workLogQuery).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to import 2025 work logs: %w", err)
	}

	// Chore assignments remapped to chore_id + 10000
	assignmentQuery := `
		INSERT INTO chore_assignments (
			user_id, chore_id, created, acked, refused, timeouted, volunteered,
			deadline_reminded, after_deadline_reminded, reminded
		)
		SELECT
			user_id, chore_id + 10000, created, acked, refused, timeouted, volunteered,
			deadline_reminded, after_deadline_reminded, reminded
		FROM db2025.chore_assignments
	`
	if err := tx.Exec(assignmentQuery).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to import 2025 chore assignments: %w", err)
	}

	// Presence logs
	presenceQuery := `
		INSERT INTO presence_logs (user_id, timestamp, event)
		SELECT user_id, timestamp, 'g::t::6.9'
		FROM db2025.presence_logs
	`
	if err := tx.Exec(presenceQuery).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to import 2025 presence logs: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit historical import: %w", err)
	}

	s.logger.Info("Successfully imported historical 2025 data", "event", "g::t::6.9")
	return nil
}
