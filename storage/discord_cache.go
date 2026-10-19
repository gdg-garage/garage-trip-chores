package storage

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const discordCacheTTL = 10 * time.Minute

// DiscordCache provides thread-safe in-memory caching for Discord API entities with a 10-minute TTL.
type DiscordCache struct {
	mu           sync.RWMutex
	roles        map[string]*discordgo.Role
	rolesAt      time.Time
	members      []*discordgo.Member
	membersById  map[string]*discordgo.Member
	membersAt    time.Time
	skills       []string
	skillsAt     time.Time
}

func NewDiscordCache() *DiscordCache {
	return &DiscordCache{
		roles:       make(map[string]*discordgo.Role),
		membersById: make(map[string]*discordgo.Member),
	}
}

// GetRoles returns cached guild roles, fetching from Discord API if expired or empty
func (c *DiscordCache) GetRoles(session *discordgo.Session, guildID string) (map[string]*discordgo.Role, error) {
	c.mu.RLock()
	if c.roles != nil && len(c.roles) > 0 && time.Since(c.rolesAt) < discordCacheTTL {
		cached := make(map[string]*discordgo.Role, len(c.roles))
		for k, v := range c.roles {
			cached[k] = v
		}
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	if session == nil || guildID == "" {
		return map[string]*discordgo.Role{}, nil
	}

	// Try reading from Discord State first if available
	if session.State != nil {
		if guild, err := session.State.Guild(guildID); err == nil && guild != nil && len(guild.Roles) > 0 {
			res := make(map[string]*discordgo.Role, len(guild.Roles))
			for _, r := range guild.Roles {
				res[r.ID] = r
			}
			c.mu.Lock()
			c.roles = res
			c.rolesAt = time.Now()
			c.mu.Unlock()
			return res, nil
		}
	}

	roles, err := session.GuildRoles(guildID)
	if err != nil {
		// On error, return stale cache if available
		c.mu.RLock()
		if len(c.roles) > 0 {
			cached := make(map[string]*discordgo.Role, len(c.roles))
			for k, v := range c.roles {
				cached[k] = v
			}
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		return nil, fmt.Errorf("failed to fetch guild roles: %w", err)
	}

	res := make(map[string]*discordgo.Role, len(roles))
	for _, r := range roles {
		res[r.ID] = r
	}

	c.mu.Lock()
	c.roles = res
	c.rolesAt = time.Now()
	c.mu.Unlock()

	return res, nil
}

// GetMembers returns cached guild members, fetching from Discord API if expired or empty
func (c *DiscordCache) GetMembers(session *discordgo.Session, guildID string) ([]*discordgo.Member, error) {
	c.mu.RLock()
	if c.members != nil && len(c.members) > 0 && time.Since(c.membersAt) < discordCacheTTL {
		cached := make([]*discordgo.Member, len(c.members))
		copy(cached, c.members)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	if session == nil || guildID == "" {
		return []*discordgo.Member{}, nil
	}

	members, err := session.GuildMembers(guildID, "", 1000)
	if err != nil {
		c.mu.RLock()
		if len(c.members) > 0 {
			cached := make([]*discordgo.Member, len(c.members))
			copy(cached, c.members)
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
		return nil, fmt.Errorf("failed to fetch guild members: %w", err)
	}

	byId := make(map[string]*discordgo.Member, len(members))
	for _, m := range members {
		if m.User != nil {
			byId[m.User.ID] = m
		}
	}

	c.mu.Lock()
	c.members = members
	c.membersById = byId
	c.membersAt = time.Now()
	c.mu.Unlock()

	cached := make([]*discordgo.Member, len(members))
	copy(cached, members)
	return cached, nil
}

// GetMember returns a single guild member, checking cached members first before querying Discord
func (c *DiscordCache) GetMember(session *discordgo.Session, guildID string, userID string) (*discordgo.Member, error) {
	c.mu.RLock()
	if time.Since(c.membersAt) < discordCacheTTL {
		if m, ok := c.membersById[userID]; ok {
			c.mu.RUnlock()
			return m, nil
		}
	}
	c.mu.RUnlock()

	// If members not loaded yet, load all members to populate cache
	members, err := c.GetMembers(session, guildID)
	if err == nil {
		for _, m := range members {
			if m.User != nil && m.User.ID == userID {
				return m, nil
			}
		}
	}

	if session == nil || guildID == "" || userID == "" {
		return nil, fmt.Errorf("member not found: %s", userID)
	}

	// Fallback to single fetch
	member, err := session.GuildMember(guildID, userID)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	if c.membersById == nil {
		c.membersById = make(map[string]*discordgo.Member)
	}
	c.membersById[userID] = member
	c.mu.Unlock()

	return member, nil
}

// GetSkills returns cached skills parsed from role prefix
func (c *DiscordCache) GetSkills(session *discordgo.Session, guildID string, prefix string) ([]string, error) {
	c.mu.RLock()
	if c.skills != nil && time.Since(c.skillsAt) < discordCacheTTL {
		res := make([]string, len(c.skills))
		copy(res, c.skills)
		c.mu.RUnlock()
		return res, nil
	}
	c.mu.RUnlock()

	roles, err := c.GetRoles(session, guildID)
	if err != nil {
		return nil, err
	}

	skills := []string{}
	seen := make(map[string]bool)
	for _, role := range roles {
		if role != nil && strings.HasPrefix(role.Name, prefix) {
			sName := strings.TrimSpace(role.Name[len(prefix):])
			if sName != "" && !seen[sName] {
				seen[sName] = true
				skills = append(skills, sName)
			}
		}
	}
	sort.Strings(skills)

	c.mu.Lock()
	c.skills = skills
	c.skillsAt = time.Now()
	c.mu.Unlock()

	res := make([]string, len(skills))
	copy(res, skills)
	return res, nil
}
