package storage

import (
	_ "embed"
	"encoding/json"

	"gorm.io/gorm/clause"
)

//go:embed discord_user_map.json
var embeddedDiscordUserMap []byte

type staticDiscordUser struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Nick        string `json:"nick"`
	GlobalName  string `json:"global_name"`
	DisplayName string `json:"display_name"`
}

func (s *staticDiscordUser) BestName() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	if s.Nick != "" {
		return s.Nick
	}
	if s.GlobalName != "" {
		return s.GlobalName
	}
	if s.Username != "" {
		return s.Username
	}
	return s.ID
}

// SeedUserMap populates user profiles and the in-memory cache from the embedded discord_user_map.json
func (s *Storage) SeedUserMap() {
	if len(embeddedDiscordUserMap) == 0 {
		return
	}

	var userMap map[string]staticDiscordUser
	if err := json.Unmarshal(embeddedDiscordUserMap, &userMap); err != nil {
		s.logger.Warn("Failed to parse embedded discord user map", "error", err)
		return
	}

	s.userCacheMu.Lock()
	defer s.userCacheMu.Unlock()
	if s.userCache == nil {
		s.userCache = make(map[string]User)
	}

	for id, u := range userMap {
		bestName := u.BestName()
		handle := u.Username
		if handle == "" {
			handle = bestName
		}

		s.userCache[id] = User{
			DiscordId: id,
			Handle:    handle,
			Name:      bestName,
		}

		var existing UserProfile
		if err := s.db.Where("discord_id = ?", id).First(&existing).Error; err != nil {
			p := UserProfile{
				DiscordId:     id,
				Name:          bestName,
				DiscordHandle: handle,
			}
			_ = s.db.Create(&p).Error
		} else if existing.Name == "" || existing.Name == id {
			existing.Name = bestName
			if existing.DiscordHandle == "" {
				existing.DiscordHandle = handle
			}
			_ = s.db.Save(&existing).Error
		}
	}
	s.logger.Debug("Embedded discord user map seeded", "count", len(userMap))
}

// SyncDiscordUsers fetches all guild members from Discord, computes their best display names,
// and synchronizes them to the in-memory cache and SQLite user_profiles table.
func (s *Storage) SyncDiscordUsers() error {
	if s.discord == nil || s.conf.DiscordGuildId == "" || s.conf.DiscordGuildId == "???" {
		return nil
	}

	rolesMap, err := s.getGuildRolesMap()
	if err != nil {
		s.logger.Warn("Failed to fetch guild roles for user sync", "error", err)
	}

	members, err := s.discord.GuildMembers(s.conf.DiscordGuildId, "", 1000)
	if err != nil {
		s.logger.Warn("Failed to fetch guild members from Discord", "guild_id", s.conf.DiscordGuildId, "error", err)
		return err
	}

	s.userCacheMu.Lock()
	defer s.userCacheMu.Unlock()
	if s.userCache == nil {
		s.userCache = make(map[string]User)
	}

	for _, m := range members {
		if m.User == nil {
			continue
		}
		id := m.User.ID
		handle := m.User.Username

		bestName := m.User.Username
		if m.User.GlobalName != "" {
			bestName = m.User.GlobalName
		}
		if m.Nick != "" {
			bestName = m.Nick
		}

		var caps []string
		isPresent := false
		for _, roleId := range m.Roles {
			if role, ok := rolesMap[roleId]; ok {
				if role.Name == s.conf.PresentRole {
					isPresent = true
				}
				if s.isRoleSkill(role) {
					caps = append(caps, role.Name[len(s.conf.SkillPrefix):])
				}
			}
		}

		s.userCache[id] = User{
			DiscordId:    id,
			Handle:       handle,
			Name:         bestName,
			Capabilities: caps,
			IsPresent:    isPresent,
		}

		p := UserProfile{
			DiscordId:     id,
			Name:          bestName,
			DiscordHandle: handle,
		}
		_ = s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "discord_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name", "discord_handle", "updated_at"}),
		}).Save(&p).Error
	}

	s.logger.Info("Discord users synchronized successfully", "count", len(members))
	return nil
}

// ResolveUserName returns the human display name for a Discord ID, checking in-memory cache,
// database, embedded static map, and falling back to a Discord API query.
func (s *Storage) ResolveUserName(discordId string) string {
	if discordId == "" {
		return ""
	}

	// 1. Check in-memory cache
	s.userCacheMu.RLock()
	if u, ok := s.userCache[discordId]; ok && u.Name != "" && u.Name != discordId {
		s.userCacheMu.RUnlock()
		return u.Name
	}
	s.userCacheMu.RUnlock()

	// 2. Check user_profiles table in DB
	var p UserProfile
	if err := s.db.Where("discord_id = ?", discordId).First(&p).Error; err == nil {
		if p.Name != "" && p.Name != discordId {
			s.userCacheMu.Lock()
			s.userCache[discordId] = User{
				DiscordId: discordId,
				Name:      p.Name,
				Handle:    p.DiscordHandle,
			}
			s.userCacheMu.Unlock()
			return p.Name
		}
		if p.DiscordHandle != "" {
			return p.DiscordHandle
		}
	}

	// 3. Fallback to Discord API on demand
	if s.discord != nil && s.conf.DiscordGuildId != "" && s.conf.DiscordGuildId != "???" {
		member, err := s.discord.GuildMember(s.conf.DiscordGuildId, discordId)
		if err == nil && member != nil && member.User != nil {
			bestName := member.User.Username
			if member.User.GlobalName != "" {
				bestName = member.User.GlobalName
			}
			if member.Nick != "" {
				bestName = member.Nick
			}
			handle := member.User.Username

			s.userCacheMu.Lock()
			s.userCache[discordId] = User{
				DiscordId: discordId,
				Name:      bestName,
				Handle:    handle,
			}
			s.userCacheMu.Unlock()

			_ = s.UpsertProfile(discordId, bestName, handle)
			return bestName
		}
	}

	return discordId
}

// GetAllGuildUsers returns all known users from the in-memory cache, DB profiles, and Discord guild.
func (s *Storage) GetAllGuildUsers() ([]User, error) {
	s.userCacheMu.Lock()
	if s.userCache == nil {
		s.userCache = make(map[string]User)
	}

	var profiles []UserProfile
	_ = s.db.Find(&profiles).Error
	for _, p := range profiles {
		if _, ok := s.userCache[p.DiscordId]; !ok {
			name := p.Name
			if name == "" {
				name = p.DiscordHandle
			}
			if name == "" {
				name = p.DiscordId
			}
			s.userCache[p.DiscordId] = User{
				DiscordId: p.DiscordId,
				Name:      name,
				Handle:    p.DiscordHandle,
			}
		}
	}
	s.userCacheMu.Unlock()

	s.userCacheMu.RLock()
	defer s.userCacheMu.RUnlock()
	users := make([]User, 0, len(s.userCache))
	for _, u := range s.userCache {
		users = append(users, u)
	}
	return users, nil
}
