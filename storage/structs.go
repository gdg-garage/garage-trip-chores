package storage

import (
	"strings"
	"time"
)

type User struct {
	DiscordId    string
	Handle       string
	Capabilities []string
}

type Chore struct {
	ID                    uint
	Name                  string
	NecessaryCapabilities string // Comma separated list of capabilities
	NecessaryWorkers      uint
	EstimatedTimeMin      uint
	AssignmentTimeoutMin  uint
	CreatorId             string // Discord ID of the user who created the chore
	MessageId             string // ID of the message in Discord where the chore was posted
	Created               time.Time
	Completed             *time.Time
	Cancelled             *time.Time
	Deadline              *time.Time
	necessaryCapabilities []string
}

func (c *Chore) GetCapabilities() []string {
	c.necessaryCapabilities = strings.Split(c.NecessaryCapabilities, ",")
	return c.necessaryCapabilities
}

func (c *Chore) SetCapabilities(capabilities []string) {
	c.necessaryCapabilities = capabilities
	c.NecessaryCapabilities = strings.Join(capabilities, ",")
}

func (c *Chore) Complete() {
	now := time.Now()
	c.Completed = &now
}

func (c *Chore) Cancel() {
	now := time.Now()
	c.Cancelled = &now
}

type WorkLog struct {
	ID           uint
	UserId       string
	ChoreId      uint
	Chore        Chore
	TimeSpentMin uint
}

type ChoreAssignment struct {
	ID        uint
	UserId    string
	ChoreId   uint
	Chore     Chore
	Created   time.Time
	Acked     *time.Time
	Refused   *time.Time
	Timeouted *time.Time
}

func (ca *ChoreAssignment) Ack() {
	now := time.Now()
	ca.Acked = &now
}

func (ca *ChoreAssignment) Refuse() {
	now := time.Now()
	ca.Refused = &now
}

func (ca *ChoreAssignment) Timeout() {
	now := time.Now()
	ca.Timeouted = &now
}

type ChoreStats struct {
	Count    uint
	TotalMin uint
}

type ChoreStatsWithCapabilities struct {
	ChoreStats
	CapabilitiesMatched uint
}

type UserChoreStats map[string]ChoreStats

func (st UserChoreStats) Add(other UserChoreStats) UserChoreStats {
	sum := UserChoreStats{}
	for user, stats := range st {
		if _, exists := sum[user]; !exists {
			sum[user] = ChoreStats{}
		}
		temp := sum[user]
		temp.Count += stats.Count
		temp.TotalMin += stats.TotalMin
		sum[user] = temp
	}
	for user, stats := range other {
		if _, exists := sum[user]; !exists {
			sum[user] = ChoreStats{}
		}
		temp := sum[user]
		temp.Count += stats.Count
		temp.TotalMin += stats.TotalMin
		sum[user] = temp
	}
	return sum
}
