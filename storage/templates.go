package storage

import (
	"time"

	"gorm.io/gorm/clause"
)

func (s *Storage) SeedDefaultTemplates() error {
	var count int64
	s.db.Model(&ChoreTemplate{}).Count(&count)
	if count > 0 {
		return nil
	}

	defaults := []ChoreTemplate{
		{
			Key:                   "floor-sweep-dry",
			Name:                  "Floor sweep (dry)",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      15,
			AssignmentTimeoutMin:  15,
			NecessaryCapabilities: "[]",
			ScalesWithHeadcount:   false,
			PerPersonMin:          0,
			SortOrder:             0,
		},
		{
			Key:                   "floor-sweep-water",
			Name:                  "Floor sweep (with water)",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      25,
			AssignmentTimeoutMin:  15,
			NecessaryCapabilities: "[]",
			ScalesWithHeadcount:   false,
			PerPersonMin:          0,
			SortOrder:             1,
		},
		{
			Key:                   "dishwasher-load",
			Name:                  "Load the dishwasher",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      10,
			AssignmentTimeoutMin:  15,
			NecessaryCapabilities: "[]",
			ScalesWithHeadcount:   true,
			PerPersonMin:          1,
			SortOrder:             2,
		},
		{
			Key:                   "dishwasher-unload",
			Name:                  "Unload the dishwasher",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      8,
			AssignmentTimeoutMin:  15,
			NecessaryCapabilities: "[]",
			ScalesWithHeadcount:   true,
			PerPersonMin:          1,
			SortOrder:             3,
		},
		{
			Key:                   "take-out-bin",
			Name:                  "Take out the bin",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      5,
			AssignmentTimeoutMin:  10,
			NecessaryCapabilities: "[]",
			ScalesWithHeadcount:   false,
			PerPersonMin:          0,
			SortOrder:             4,
		},
		{
			Key:                   "grilling",
			Name:                  "Grilling",
			NecessaryWorkers:      2,
			EstimatedTimeMin:      60,
			AssignmentTimeoutMin:  20,
			NecessaryCapabilities: "[\"grilling\"]",
			ScalesWithHeadcount:   true,
			PerPersonMin:          3,
			SortOrder:             5,
		},
		{
			Key:                   "kitchen-sweep",
			Name:                  "Kitchen sweep",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      20,
			AssignmentTimeoutMin:  15,
			NecessaryCapabilities: "[]",
			ScalesWithHeadcount:   false,
			PerPersonMin:          0,
			SortOrder:             6,
		},
		{
			Key:                   "water-pipe-cleaning",
			Name:                  "Water pipe (shisha) cleaning",
			NecessaryWorkers:      1,
			EstimatedTimeMin:      45,
			AssignmentTimeoutMin:  20,
			NecessaryCapabilities: "[\"hookah_master\"]",
			ScalesWithHeadcount:   false,
			PerPersonMin:          0,
			SortOrder:             7,
		},
		{
			Key:                   "cooking",
			Name:                  "Cooking",
			NecessaryWorkers:      2,
			EstimatedTimeMin:      90,
			AssignmentTimeoutMin:  20,
			NecessaryCapabilities: "[\"cooking\"]",
			ScalesWithHeadcount:   true,
			PerPersonMin:          4,
			SortOrder:             8,
		},
	}

	for _, t := range defaults {
		if err := s.db.Create(&t).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) GetTemplates() ([]ChoreTemplate, error) {
	var list []ChoreTemplate
	err := s.db.Order("sort_order asc, name asc").Find(&list).Error
	return list, err
}

func (s *Storage) GetTemplate(key string) (*ChoreTemplate, error) {
	var t ChoreTemplate
	err := s.db.Where("key = ?", key).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Storage) UpsertTemplate(t *ChoreTemplate) error {
	now := time.Now()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now

	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "necessary_workers", "estimated_time_min",
			"assignment_timeout_min", "necessary_capabilities",
			"scales_with_headcount", "per_person_min", "updated_at",
		}),
	}).Create(t).Error
}

func (s *Storage) DeleteTemplate(key string) error {
	return s.db.Where("key = ?", key).Delete(&ChoreTemplate{}).Error
}

func (s *Storage) UpsertProfile(discordID, name, handle string) error {
	now := time.Now()
	p := UserProfile{
		DiscordId:     discordID,
		Name:          name,
		DiscordHandle: handle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "discord_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "discord_handle", "updated_at",
		}),
	}).Create(&p).Error
}

func (s *Storage) GetProfile(discordID string) (*UserProfile, error) {
	var p UserProfile
	err := s.db.Where("discord_id = ?", discordID).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Storage) GetAllProfiles() ([]UserProfile, error) {
	var list []UserProfile
	err := s.db.Find(&list).Error
	return list, err
}
