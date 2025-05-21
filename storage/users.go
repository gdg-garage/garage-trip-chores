package storage

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm/clause"
)

func (s *Storage) GetPresentUsers() ([]User, error) {
	guild, err := s.discord.State.Guild(s.conf.DiscordGuildId)
	if err != nil {
		return nil, err
	}

	guildRolesMap := make(map[string]*discordgo.Role)
	for _, role := range guild.Roles {
		guildRolesMap[role.ID] = role
	}

	users := []User{}
	members, err := s.discord.GuildMembers(s.conf.DiscordGuildId, "", 1000)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		roles := []string{}
		isPresent := false
		for _, roleId := range member.Roles {
			if role, ok := guildRolesMap[roleId]; ok {
				if role.Name == s.conf.PresentRole {
					isPresent = true
				}
				if !strings.HasPrefix(role.Name, s.conf.SkillPrefix) {
					continue
				}
				roles = append(roles, role.Name[len(s.conf.SkillPrefix):])
			}
		}
		if !isPresent {
			continue
		}
		users = append(users, User{
			Handle:       member.User.Username,
			DiscordId:    member.User.ID,
			Capabilities: roles,
		})
	}

	return users, nil
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
