package storage

import (
	"time"

	"gorm.io/gorm/clause"
)

// SaveUserRegistrations bulk upserts registrations into the local database
func (s *Storage) SaveUserRegistrations(regs []UserRegistration) error {
	if len(regs) == 0 {
		return nil
	}

	now := time.Now()
	for i := range regs {
		regs[i].UpdatedAt = now
		if regs[i].CreatedAt.IsZero() {
			regs[i].CreatedAt = now
		}
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "discord_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"username", "event", "arrival_date", "departure_date", "cancelled",
			"paid", "food_restrictions", "children_count", "note", "updated_at",
		}),
	}).Create(&regs).Error
}

// GetUserRegistrations returns all active registrations, optionally filtered by event
func (s *Storage) GetUserRegistrations(event string) ([]UserRegistration, error) {
	var regs []UserRegistration
	query := s.db
	if event != "" {
		query = query.Where("event = ?", event)
	}
	err := query.Find(&regs).Error
	return regs, err
}

// GetUserRegistration returns the registration for a specific discord ID
func (s *Storage) GetUserRegistration(discordID string) (*UserRegistration, error) {
	var reg UserRegistration
	err := s.db.Where("discord_id = ?", discordID).First(&reg).Error
	if err != nil {
		return nil, err
	}
	return &reg, nil
}

// GetUserRegistrationsMap returns a map of discord_id -> UserRegistration
func (s *Storage) GetUserRegistrationsMap(event string) (map[string]UserRegistration, error) {
	regs, err := s.GetUserRegistrations(event)
	if err != nil {
		return nil, err
	}

	res := make(map[string]UserRegistration, len(regs))
	for _, r := range regs {
		res[r.DiscordId] = r
	}
	return res, nil
}

// SaveTripEvents bulk upserts trip events into the local database
func (s *Storage) SaveTripEvents(events []TripEvent) error {
	if len(events) == 0 {
		return nil
	}

	now := time.Now()
	for i := range events {
		events[i].UpdatedAt = now
	}

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "code"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "start_date", "end_date", "location", "description",
			"enabled", "status", "updated_at",
		}),
	}).Create(&events).Error
}

// GetTripEvents returns all known events
func (s *Storage) GetTripEvents() ([]TripEvent, error) {
	var events []TripEvent
	err := s.db.Find(&events).Error
	return events, err
}

// IsDateInEvent returns true if target date is within any trip event's duration (inclusive of start and end dates).
func (s *Storage) IsDateInEvent(date time.Time) (bool, *TripEvent, error) {
	events, err := s.GetTripEvents()
	if err != nil {
		return false, nil, err
	}

	y, m, d := date.Date()
	targetDay := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	for _, e := range events {
		if e.StartDate != nil && e.EndDate != nil {
			sy, sm, sd := e.StartDate.Date()
			startDay := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)

			ey, em, ed := e.EndDate.Date()
			endDay := time.Date(ey, em, ed, 23, 59, 59, 0, time.UTC)

			if !targetDay.Before(startDay) && !targetDay.After(endDay) {
				return true, &e, nil
			}
		}
	}
	return false, nil, nil
}

