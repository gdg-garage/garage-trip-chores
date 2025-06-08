package storage

import (
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"gorm.io/gorm/clause"
)

func (s *Storage) getGuildRolesMap() (map[string]*discordgo.Role, error) {
	guild, err := s.discord.State.Guild(s.conf.DiscordGuildId)
	if err != nil {
		return nil, err
	}

	guildRolesMap := make(map[string]*discordgo.Role)
	for _, role := range guild.Roles {
		guildRolesMap[role.ID] = role
	}
	return guildRolesMap, nil
}

func (s *Storage) isRoleSkill(role *discordgo.Role) bool {
	if role == nil {
		return false
	}
	return strings.HasPrefix(role.Name, s.conf.SkillPrefix)
}

func (s *Storage) GetSkills() ([]string, error) {
	guildRolesMap, err := s.getGuildRolesMap()
	if err != nil {
		return nil, err
	}

	skills := []string{}
	for _, role := range guildRolesMap {
		if s.isRoleSkill(role) {
			skills = append(skills, role.Name[len(s.conf.SkillPrefix):])
		}
	}
	return skills, nil
}

func (s *Storage) GetPresentUsers() ([]User, error) {
	guildRolesMap, err := s.getGuildRolesMap()
	if err != nil {
		return nil, err
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
				if !s.isRoleSkill(role) {
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

func (s *Storage) GetUserHandleByDiscordId(discordId string) (string, error) {
	member, err := s.discord.GuildMember(s.conf.DiscordGuildId, discordId)
	if err != nil {
		return "", err
	}
	if member == nil || member.User == nil {
		return "", nil
	}
	return member.User.Username, nil
}

func (s *Storage) GetUserStats() (UserChoreStats, error) {
	type result struct {
		UserId     string
		TotalTime  int
		TotalCount int
	}
	var results []result
	stats := UserChoreStats{}
	// TODO this should be based on ID
	r := s.db.Model(&WorkLog{}).Select("user_id, sum(time_spent_min) as total_time, count(*) as total_count").Group("user_id").Find(&results)
	if r.Error != nil {
		return stats, r.Error
	}
	for _, r := range results {
		stats[r.UserId] = ChoreStats{
			Count:    uint(r.TotalCount),
			TotalMin: uint(r.TotalTime),
		}
	}
	return stats, nil
}

func (s *Storage) GetAssignedStats() (UserChoreStats, error) {
	type result struct {
		UserId     string
		TotalTime  int
		TotalCount int
	}
	var results []result
	stats := UserChoreStats{}
	// TODO this should be based on ID
	r := s.db.Model(&ChoreAssignment{}).Select("user_id, sum(chores.estimated_time_min) as total_time, count(*) as total_count").Joins("left join chores on chore_assignments.chore_id = chores.id").Where("refused IS NULL and timeouted IS NULL").Group("user_id").Find(&results)
	if r.Error != nil {
		return stats, r.Error
	}
	for _, r := range results {
		stats[r.UserId] = ChoreStats{
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

func (s *Storage) AssignChore(chore Chore, userId string) (ChoreAssignment, error) {
	ChoreAssignment := ChoreAssignment{
		Chore:   chore,
		UserId:  userId,
		Created: time.Now(),
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

func (s *Storage) GetChoresAssignments() ([]ChoreAssignment, error) {
	var assignments []ChoreAssignment
	r := s.db.Preload(clause.Associations).Find(&assignments)
	return assignments, r.Error
}

func (s *Storage) GetChoreAssignments(choreId uint) ([]ChoreAssignment, error) {
	var assignments []ChoreAssignment
	r := s.db.Preload(clause.Associations).Where("chore_id = ?", choreId).Find(&assignments)
	return assignments, r.Error
}

func (s *Storage) GetChoreAssignment(choreId uint, userId string) (ChoreAssignment, error) {
	var assignment ChoreAssignment
	r := s.db.Preload(clause.Associations).Where("chore_id = ? AND user_id = ?", choreId, userId).First(&assignment)
	return assignment, r.Error
}
