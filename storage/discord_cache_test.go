package storage

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestDiscordCacheRolesAndSkills(t *testing.T) {
	cache := NewDiscordCache()

	// Pre-populate cache directly
	cache.mu.Lock()
	cache.roles = map[string]*discordgo.Role{
		"role1": {ID: "role1", Name: "skill:cooking"},
		"role2": {ID: "role2", Name: "skill:driving"},
		"role3": {ID: "role3", Name: "guest"},
	}
	cache.rolesAt = time.Now()
	cache.mu.Unlock()

	// Verify roles fetched from cache without network call
	roles, err := cache.GetRoles(nil, "guild_1")
	if err != nil {
		t.Fatalf("unexpected error getting roles: %v", err)
	}
	if len(roles) != 3 {
		t.Fatalf("expected 3 roles from cache, got %d", len(roles))
	}

	// Verify skills extraction and caching
	skills, err := cache.GetSkills(nil, "guild_1", "skill:")
	if err != nil {
		t.Fatalf("unexpected error getting skills: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d: %v", len(skills), skills)
	}
	if skills[0] != "cooking" || skills[1] != "driving" {
		t.Fatalf("unexpected skills content: %v", skills)
	}

	// Verify members cache lookup
	cache.mu.Lock()
	cache.members = []*discordgo.Member{
		{User: &discordgo.User{ID: "user_a", Username: "alicesmith"}},
	}
	cache.membersById = map[string]*discordgo.Member{
		"user_a": cache.members[0],
	}
	cache.membersAt = time.Now()
	cache.mu.Unlock()

	member, err := cache.GetMember(nil, "guild_1", "user_a")
	if err != nil {
		t.Fatalf("unexpected error getting cached member: %v", err)
	}
	if member.User.Username != "alicesmith" {
		t.Fatalf("expected alicesmith, got %s", member.User.Username)
	}
}
