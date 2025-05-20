package storage

import (
	"time"

	"gorm.io/gorm/clause"
)

func (s *Storage) addCapability(handle string, capability string) error {
	// TODO: load from Discord
	return nil
}

func (s *Storage) GetPresentUsers() ([]User, error) {
	// TODO: load from Discord
	return nil, nil
}

func (s *Storage) GetUserStats() (UserChoreStats, error) {
	type result struct {
		UserHandle string
		TotalTime  int
		TotalCount int
	}
	var results []result
	stats := UserChoreStats{}
	r := s.db.Model(&WorkLog{}).Select("user_handle, sum(time_spent_min) as total_time, count(*) as total_count").Group("user_handle").Find(&results)
	if r.Error != nil {
		return stats, r.Error
	}
	for _, r := range results {
		stats[r.UserHandle] = ChoreStats{
			Count:    uint(r.TotalCount),
			TotalMin: uint(r.TotalTime),
		}
	}
	return stats, nil
}

func (s *Storage) GetAssignedStats() (UserChoreStats, error) {
	type result struct {
		UserHandle string
		TotalTime  int
		TotalCount int
	}
	var results []result
	stats := UserChoreStats{}
	r := s.db.Model(&ChoreAssignment{}).Select("user_handle, sum(chores.estimated_time_min) as total_time, count(*) as total_count").Joins("left join chores on chore_assignments.chore_id = chores.id").Where("refused IS NULL and timeouted IS NULL").Group("user_handle").Find(&results)
	if r.Error != nil {
		return stats, r.Error
	}
	for _, r := range results {
		stats[r.UserHandle] = ChoreStats{
			Count:    uint(r.TotalCount),
			TotalMin: uint(r.TotalTime),
		}
	}
	return stats, nil
}

func (s *Storage) GetTotalChoreStats() (UserChoreStats, error) {
	userStats, err := s.GetUserStats()
	if err != nil {
		return userStats, err
	}

	userAssignedStats, err := s.GetAssignedStats()
	if err != nil {
		return userStats, err
	}

	return userStats.Add(userAssignedStats), nil
}

func (s *Storage) AssignChore(chore Chore, userHandle string) (ChoreAssignment, error) {
	ChoreAssignment := ChoreAssignment{
		Chore:      chore,
		UserHandle: userHandle,
		Created:    time.Now(),
	}
	return s.SaveChoreAssignment(ChoreAssignment)
}

func (s *Storage) SaveChoreAssignment(ca ChoreAssignment) (ChoreAssignment, error) {
	r := s.db.Save(&ca)
	return ca, r.Error
}

func (s *Storage) SaveChoreAssignments(assignments []ChoreAssignment) ([]ChoreAssignment, error) {
	if len(assignments) == 0 {
		return assignments, nil
	}
	r := s.db.Create(&assignments)
	return assignments, r.Error
}

func (s *Storage) GetChoreAssignments() ([]ChoreAssignment, error) {
	var assignments []ChoreAssignment
	r := s.db.Preload(clause.Associations).Find(&assignments)
	return assignments, r.Error
}
