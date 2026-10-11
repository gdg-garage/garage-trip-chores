package storage

import (
	"encoding/json"
	"strings"
	"time"
)

type User struct {
	DiscordId    string
	Handle       string
	Name         string
	Capabilities []string
	IsPresent    bool
	IsAdmin      bool
}

type Chore struct {
	ID                    uint
	Name                  string
	NecessaryCapabilities string // Comma separated list of capabilities
	NecessaryWorkers      uint
	EstimatedTimeMin      uint
	AssignmentTimeoutMin  uint
	CreatorId             string // Discord ID of the user who created the chore
	AssigneeId            string // Discord ID of the directly assigned user (optional)
	MessageId             string // ID of the message in Discord where the chore was posted
	Created               time.Time
	Completed             *time.Time
	Cancelled             *time.Time
	Deadline              *time.Time
	DelayMin              uint
	necessaryCapabilities []string
	AfterDeadlineReminded bool
	SelfReported          bool
	Draft                 bool `gorm:"default:false"`
	TemplateKey           string `json:"template_key,omitempty"`
}

func (c *Chore) GetCapabilities() []string {
	if c.NecessaryCapabilities == "" {
		return []string{}
	}
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
	SelfReported bool
}

type ChoreAssignment struct {
	ID                    uint
	UserId                string
	ChoreId               uint
	Chore                 Chore
	Created               time.Time
	Acked                 *time.Time
	Refused               *time.Time
	Timeouted             *time.Time
	Volunteered           bool
	DeadlineReminded      bool
	AfterDeadlineReminded bool
	Reminded              bool
}

func (ca *ChoreAssignment) Ack() {
	now := time.Now()
	ca.Acked = &now
	ca.Refused = nil
	ca.Timeouted = nil
}

func (ca *ChoreAssignment) Refuse() {
	now := time.Now()
	ca.Refused = &now
	ca.Acked = nil
	ca.Timeouted = nil
}

func (ca *ChoreAssignment) Timeout() {
	now := time.Now()
	ca.Timeouted = &now
	ca.Acked = nil
	ca.Refused = nil
}

type ChoreStats struct {
	Count    float64
	TotalMin float64
}

type ChoreStatsWithCapabilities struct {
	ChoreStats
	CapabilitiesMatched uint
	PresentTicks        int
	OnCooldown          bool
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

type PresenceLog struct {
	ID        uint
	UserId    string
	Timestamp time.Time
}

type LLMSummaryLog struct {
	ID         uint      `gorm:"primaryKey"`
	RunAt      time.Time `gorm:"index"`
	StatsJSON  string    `gorm:"type:text"`
	Summary    string    `gorm:"type:text"`
	TasksCount int
	Status     string
}

type TasksActivity struct {
	CreatedChores      []Chore
	CompletedChores    []Chore
	CancelledChores    []Chore
	UpdatedAssignments []ChoreAssignment
	ActiveChores       []Chore
	WorkLogs           []WorkLog
}

func (a *TasksActivity) HasActivity() bool {
	if a == nil {
		return false
	}
	return len(a.CreatedChores) > 0 ||
		len(a.CompletedChores) > 0 ||
		len(a.CancelledChores) > 0 ||
		len(a.UpdatedAssignments) > 0 ||
		len(a.WorkLogs) > 0
}

func (a *TasksActivity) TotalActivityCount() int {
	if a == nil {
		return 0
	}
	return len(a.CreatedChores) + len(a.CompletedChores) + len(a.CancelledChores) + len(a.UpdatedAssignments) + len(a.WorkLogs)
}

type DelayedTask struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	ChoreID    uint       `gorm:"not null;index" json:"chore_id"`
	Chore      Chore      `gorm:"foreignKey:ChoreID" json:"chore"`
	PublishAt  time.Time  `gorm:"not null;index" json:"publish_at"`
	DelayMin   uint       `gorm:"not null" json:"delay_min"`
	CreatedAt  time.Time  `json:"created_at"`
	ExecutedAt *time.Time `json:"executed_at,omitempty"`
}

type ChoreTemplate struct {
	Key                   string    `gorm:"primaryKey" json:"key"`
	Name                  string    `gorm:"not null" json:"name"`
	NecessaryWorkers      uint      `gorm:"not null;default:1" json:"necessary_workers"`
	EstimatedTimeMin      uint      `gorm:"not null;default:10" json:"estimated_time_min"`
	AssignmentTimeoutMin uint      `gorm:"not null;default:15" json:"assignment_timeout_min"`
	NecessaryCapabilities string    `gorm:"not null;default:'[]'" json:"necessary_capabilities"`
	ScalesWithHeadcount   bool      `gorm:"not null;default:false" json:"scales_with_headcount"`
	PerPersonMin          uint      `gorm:"not null;default:0" json:"per_person_min"`
	SortOrder             int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type UserProfile struct {
	DiscordId     string    `gorm:"primaryKey" json:"discord_id"`
	Name          string    `gorm:"not null" json:"name"`
	DiscordHandle string    `gorm:"not null" json:"discord_handle"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ScheduledTask struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	Name                  string     `gorm:"not null" json:"name"`
	Description           string     `json:"description,omitempty"`
	CronExpr              string     `gorm:"not null" json:"cron_expr"`
	NecessaryWorkers      uint       `gorm:"not null;default:1" json:"necessary_workers"`
	EstimatedTimeMin      uint       `gorm:"not null;default:15" json:"estimated_time_min"`
	AssignmentTimeoutMin  uint       `gorm:"not null;default:15" json:"assignment_timeout_min"`
	NecessaryCapabilities string     `gorm:"not null;default:'[]'" json:"necessary_capabilities"`
	AssigneeId            string     `json:"assignee_id,omitempty"`
	CreatorId             string     `gorm:"not null" json:"creator_id"`
	CreatorName           string     `json:"creator_name,omitempty"`
	Enabled               bool       `gorm:"not null;default:true" json:"enabled"`
	TemplateKey           string     `json:"template_key,omitempty"`
	LastRunAt             *time.Time `json:"last_run_at,omitempty"`
	NextRunAt             *time.Time `json:"next_run_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (t *ScheduledTask) GetCapabilities() []string {
	if t.NecessaryCapabilities == "" || t.NecessaryCapabilities == "[]" {
		return []string{}
	}
	if strings.HasPrefix(t.NecessaryCapabilities, "[") {
		var caps []string
		if err := json.Unmarshal([]byte(t.NecessaryCapabilities), &caps); err == nil {
			return caps
		}
	}
	return strings.Split(t.NecessaryCapabilities, ",")
}

type ShiftDefinition struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	Name                  string    `gorm:"not null" json:"name"`
	Category              string    `gorm:"not null;default:'General'" json:"category"`
	Description           string    `json:"description,omitempty"`
	DocURL                string    `json:"doc_url,omitempty"`
	WorkersCount          uint      `gorm:"not null;default:1" json:"workers_count"`
	ShiftLengthMin        uint      `gorm:"not null;default:120" json:"shift_length_min"`
	ActiveHoursStart      string    `gorm:"not null;default:'08:00'" json:"active_hours_start"`
	ActiveHoursEnd        string    `gorm:"not null;default:'22:00'" json:"active_hours_end"`
	StandbyRatio          float64   `gorm:"not null;default:0.10" json:"standby_ratio"`
	NecessaryCapabilities string    `gorm:"not null;default:'[]'" json:"necessary_capabilities"`
	Enabled               bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (d *ShiftDefinition) GetCapabilities() []string {
	if d.NecessaryCapabilities == "" || d.NecessaryCapabilities == "[]" {
		return []string{}
	}
	if strings.HasPrefix(d.NecessaryCapabilities, "[") {
		var caps []string
		if err := json.Unmarshal([]byte(d.NecessaryCapabilities), &caps); err == nil {
			return caps
		}
	}
	return strings.Split(d.NecessaryCapabilities, ",")
}

type Shift struct {
	ID                    uint              `gorm:"primaryKey" json:"id"`
	DefinitionID          uint              `gorm:"not null;index" json:"definition_id"`
	Definition            ShiftDefinition   `gorm:"foreignKey:DefinitionID" json:"definition"`
	Name                  string            `gorm:"not null" json:"name"`
	Category              string            `gorm:"not null;default:'General'" json:"category"`
	DocURL                string            `json:"doc_url,omitempty"`
	Date                  string            `gorm:"not null;index" json:"date"` // YYYY-MM-DD
	StartTime             time.Time         `gorm:"not null;index" json:"start_time"`
	EndTime               time.Time         `gorm:"not null" json:"end_time"`
	WorkersCount          uint              `gorm:"not null;default:1" json:"workers_count"`
	StandbyRatio          float64           `gorm:"not null;default:0.10" json:"standby_ratio"`
	Frozen                bool              `gorm:"not null;default:false" json:"frozen"`
	Status                string            `gorm:"not null;default:'scheduled'" json:"status"` // scheduled, active, completed, cancelled
	MessageId             string            `json:"message_id,omitempty"`
	Assignments           []ShiftAssignment `gorm:"foreignKey:ShiftID" json:"assignments"`
	CreatedAt             time.Time         `json:"created_at"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

func (s *Shift) DurationMin() uint {
	if s.EndTime.After(s.StartTime) {
		return uint(s.EndTime.Sub(s.StartTime).Minutes())
	}
	return 0
}

func (s *Shift) StandbyCreditedMin() float64 {
	ratio := s.StandbyRatio
	if ratio <= 0 {
		ratio = 0.10
	}
	return float64(s.DurationMin()) * ratio
}

type ShiftAssignment struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	ShiftID            uint      `gorm:"not null;index" json:"shift_id"`
	Shift              Shift     `gorm:"foreignKey:ShiftID" json:"shift"`
	UserId             string    `gorm:"not null;index" json:"user_id"`
	StandbyCreditedMin float64   `gorm:"not null;default:0" json:"standby_credited_min"`
	Status             string    `gorm:"not null;default:'assigned'" json:"status"` // assigned, completed, swapped, refused, dropped
	Notes              string    `json:"notes,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ShiftSwapRequest struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	FromAssignmentID uint       `gorm:"not null;index" json:"from_assignment_id"`
	FromUserId       string     `gorm:"not null;index" json:"from_user_id"`
	ToAssignmentID   *uint      `json:"to_assignment_id,omitempty"`
	ToUserId         string     `gorm:"not null;index" json:"to_user_id"`
	Status           string     `gorm:"not null;default:'pending'" json:"status"` // pending, accepted, rejected, cancelled
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
}


