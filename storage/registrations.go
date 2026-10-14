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
