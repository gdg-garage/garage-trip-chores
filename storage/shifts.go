package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// ShiftDefinition operations

func (s *Storage) CreateShiftDefinition(def ShiftDefinition) (ShiftDefinition, error) {
	now := time.Now()
	def.CreatedAt = now
	def.UpdatedAt = now
	if def.WorkersCount == 0 {
		def.WorkersCount = 1
	}
	if def.ShiftLengthMin == 0 {
		def.ShiftLengthMin = 120
	}
	if def.ActiveHoursStart == "" {
		def.ActiveHoursStart = "08:00"
	}
	if def.ActiveHoursEnd == "" {
		def.ActiveHoursEnd = "22:00"
	}
	if def.StandbyRatio <= 0 {
		def.StandbyRatio = 0.10
	}
	if def.Category == "" {
		def.Category = "General"
	}
	if def.NecessaryCapabilities == "" {
		def.NecessaryCapabilities = "[]"
	}
	r := s.db.Create(&def)
	return def, r.Error
}

func (s *Storage) GetShiftDefinitions() ([]ShiftDefinition, error) {
	var list []ShiftDefinition
	r := s.db.Order("category asc, name asc").Find(&list)
	return list, r.Error
}

func (s *Storage) GetActiveShiftDefinitions() ([]ShiftDefinition, error) {
	var list []ShiftDefinition
	r := s.db.Where("enabled = ?", true).Order("category asc, name asc").Find(&list)
	return list, r.Error
}

func (s *Storage) GetShiftDefinition(id uint) (*ShiftDefinition, error) {
	var def ShiftDefinition
	r := s.db.First(&def, id)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &def, nil
}

func (s *Storage) UpdateShiftDefinition(def ShiftDefinition) error {
	def.UpdatedAt = time.Now()
	return s.db.Model(&ShiftDefinition{}).
		Where("id = ?", def.ID).
		Updates(map[string]any{
			"name":                   def.Name,
			"category":               def.Category,
			"description":            def.Description,
			"doc_url":                def.DocURL,
			"workers_count":          def.WorkersCount,
			"shift_length_min":       def.ShiftLengthMin,
			"active_hours_start":      def.ActiveHoursStart,
			"active_hours_end":        def.ActiveHoursEnd,
			"standby_ratio":          def.StandbyRatio,
			"necessary_capabilities": def.NecessaryCapabilities,
			"enabled":                def.Enabled,
			"updated_at":             def.UpdatedAt,
		}).Error
}

func (s *Storage) DeleteShiftDefinition(id uint) error {
	return s.db.Delete(&ShiftDefinition{}, id).Error
}

// Shift operations

func (s *Storage) CreateShift(shift Shift) (Shift, error) {
	now := time.Now()
	shift.CreatedAt = now
	shift.UpdatedAt = now
	if shift.Status == "" {
		shift.Status = "scheduled"
	}
	if shift.StandbyRatio <= 0 {
		shift.StandbyRatio = 0.10
	}
	if shift.WorkersCount == 0 {
		shift.WorkersCount = 1
	}
	r := s.db.Create(&shift)
	if r.Error == nil && s.Events != nil {
		s.Events.Publish(Event{
			Type:  ShiftCreated,
			Shift: &shift,
		})
	}
	return shift, r.Error
}

func (s *Storage) GetShifts(date string) ([]Shift, error) {
	var list []Shift
	query := s.db.Preload("Definition").Preload("Assignments").Order("start_time asc, id asc")
	if date != "" {
		query = query.Where("date = ?", date)
	}
	r := query.Find(&list)
	return list, r.Error
}

func (s *Storage) GetUpcomingShifts(from time.Time) ([]Shift, error) {
	var list []Shift
	r := s.db.Preload("Definition").Preload("Assignments").
		Where("end_time >= ? AND status != ?", from, "cancelled").
		Order("start_time asc").
		Find(&list)
	return list, r.Error
}

func (s *Storage) GetShift(id uint) (*Shift, error) {
	var shift Shift
	r := s.db.Preload("Definition").Preload("Assignments").First(&shift, id)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &shift, nil
}

func (s *Storage) UpdateShift(shift Shift) error {
	shift.UpdatedAt = time.Now()
	err := s.db.Save(&shift).Error
	if err == nil && s.Events != nil {
		s.Events.Publish(Event{
			Type:  ShiftUpdated,
			Shift: &shift,
		})
	}
	return err
}

func (s *Storage) DeleteShift(id uint) error {
	_ = s.db.Where("shift_id = ?", id).Delete(&ShiftAssignment{})
	return s.db.Delete(&Shift{}, id).Error
}

func (s *Storage) FreezeShiftsForDate(date string) error {
	return s.db.Model(&Shift{}).
		Where("date = ?", date).
		Update("frozen", true).Error
}

// ShiftAssignment operations

func (s *Storage) CreateShiftAssignment(assignment ShiftAssignment) (ShiftAssignment, error) {
	now := time.Now()
	assignment.CreatedAt = now
	assignment.UpdatedAt = now
	if assignment.Status == "" {
		assignment.Status = "assigned"
	}
	r := s.db.Create(&assignment)
	if r.Error == nil && s.Events != nil {
		s.Events.Publish(Event{
			Type: ShiftAssigned,
		})
	}
	return assignment, r.Error
}

func (s *Storage) GetShiftAssignments(shiftID uint) ([]ShiftAssignment, error) {
	var list []ShiftAssignment
	r := s.db.Where("shift_id = ?", shiftID).Find(&list)
	return list, r.Error
}

func (s *Storage) GetShiftAssignment(id uint) (*ShiftAssignment, error) {
	var assignment ShiftAssignment
	r := s.db.Preload("Shift").Preload("Shift.Definition").First(&assignment, id)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &assignment, nil
}

func (s *Storage) GetUserShiftAssignments(userID string) ([]ShiftAssignment, error) {
	var list []ShiftAssignment
	r := s.db.Preload("Shift").Preload("Shift.Definition").
		Where("user_id = ?", userID).
		Order("created_at desc").
		Find(&list)
	return list, r.Error
}

func (s *Storage) UpdateShiftAssignment(assignment ShiftAssignment) error {
	assignment.UpdatedAt = time.Now()
	return s.db.Save(&assignment).Error
}

func (s *Storage) DeleteShiftAssignment(id uint) error {
	return s.db.Delete(&ShiftAssignment{}, id).Error
}

// GetShiftStandbyStats aggregates standby minutes and counts by user.
// Includes completed assignments and active assigned ones.
func (s *Storage) GetShiftStandbyStats() (map[string]ChoreStats, error) {
	type Result struct {
		UserId   string
		Count    float64
		TotalMin float64
	}
	var results []Result

	// Sum standby_credited_min from shift_assignments where status != 'cancelled' AND status != 'refused'
	r := s.db.Model(&ShiftAssignment{}).
		Select("user_id, count(*) as count, coalesce(sum(standby_credited_min), 0) as total_min").
		Where("status NOT IN ('refused', 'dropped', 'swapped')").
		Group("user_id").
		Scan(&results)

	if r.Error != nil {
		return nil, r.Error
	}

	stats := make(map[string]ChoreStats)
	for _, res := range results {
		stats[res.UserId] = ChoreStats{
			Count:    res.Count,
			TotalMin: res.TotalMin,
		}
	}
	return stats, nil
}

// ShiftSwapRequest operations

func (s *Storage) CreateShiftSwapRequest(req ShiftSwapRequest) (ShiftSwapRequest, error) {
	now := time.Now()
	req.CreatedAt = now
	req.UpdatedAt = now
	req.Status = "pending"
	r := s.db.Create(&req)
	return req, r.Error
}

func (s *Storage) GetShiftSwapRequest(id uint) (*ShiftSwapRequest, error) {
	var req ShiftSwapRequest
	r := s.db.First(&req, id)
	if r.Error != nil {
		if errors.Is(r.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, r.Error
	}
	return &req, nil
}

func (s *Storage) GetPendingSwapRequestsForUser(userID string) ([]ShiftSwapRequest, error) {
	var list []ShiftSwapRequest
	r := s.db.Where("to_user_id = ? AND status = 'pending'", userID).
		Order("created_at desc").
		Find(&list)
	return list, r.Error
}

func (s *Storage) UpdateShiftSwapRequest(req ShiftSwapRequest) error {
	req.UpdatedAt = time.Now()
	return s.db.Save(&req).Error
}
