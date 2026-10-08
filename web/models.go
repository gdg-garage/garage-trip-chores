package web

import "time"

type ClaimerView struct {
	DiscordId string `json:"discord_id"`
	Name      string `json:"name"`
}

type ChoreView struct {
	ID                    uint          `json:"id"`
	Name                  string        `json:"name"`
	NecessaryWorkers      uint          `json:"necessary_workers"`
	EstimatedTimeMin      uint          `json:"estimated_time_min"`
	AssignmentTimeoutMin  uint          `json:"assignment_timeout_min"`
	NecessaryCapabilities []string      `json:"necessary_capabilities"`
	Deadline              *time.Time    `json:"deadline,omitempty"`
	MinutesToDeadline     *int          `json:"minutes_to_deadline,omitempty"`
	Completed             *time.Time    `json:"completed,omitempty"`
	Cancelled             *time.Time    `json:"cancelled,omitempty"`
	Created               time.Time     `json:"created"`
	DelayMin              uint          `json:"delay_min"`
	PublishAt             *time.Time    `json:"publish_at,omitempty"`
	IsDelayed             bool          `json:"is_delayed"`
	MinutesToPublish      *int          `json:"minutes_to_publish,omitempty"`
	SelfReported          bool          `json:"self_reported"`
	CreatorId             string        `json:"creator_id,omitempty"`
	CreatorName           string        `json:"creator_name,omitempty"`
	Worklogs              []WorkLogView `json:"worklogs"`
	WorkedMinTotal        uint          `json:"worked_min_total"`
	Size                  string        `json:"size"`
	Urgent                bool          `json:"urgent"`
	Spiciness             int           `json:"spiciness"`
	TemplateKey           string        `json:"template_key,omitempty"`
	Claimers              []ClaimerView `json:"claimers"`
	ClaimedCount          int           `json:"claimed_count"`
	FullyClaimed          bool          `json:"fully_claimed"`
	Active                bool          `json:"active"`
	TotalTimeMin          uint          `json:"total_time_min,omitempty"`
}

type WorkLogView struct {
	ChoreId      uint   `json:"chore_id"`
	UserId       string `json:"user_id"`
	TimeSpentMin uint   `json:"time_spent_min"`
	SelfReported bool   `json:"self_reported"`
}

type ChoreCreateIn struct {
	Name                  string   `json:"name"`
	EstimatedTimeMin      uint     `json:"estimated_time_min"`
	NecessaryWorkers      uint     `json:"necessary_workers"`
	AssignmentTimeoutMin  uint     `json:"assignment_timeout_min"`
	NecessaryCapabilities []string `json:"necessary_capabilities"`
	DelayMin              uint     `json:"delay_min"`
	SelfReported          bool     `json:"self_reported"`
	AssigneeId            string   `json:"assignee_id,omitempty"`
	CreatorId             string   `json:"creator_id"`
	TemplateKey           string   `json:"template_key"`
}

type TemplateIn struct {
	Name                  string   `json:"name"`
	NecessaryWorkers      uint     `json:"necessary_workers"`
	EstimatedTimeMin      uint     `json:"estimated_time_min"`
	AssignmentTimeoutMin  uint     `json:"assignment_timeout_min"`
	NecessaryCapabilities []string `json:"necessary_capabilities"`
	ScalesWithHeadcount   bool     `json:"scales_with_headcount"`
	PerPersonMin          uint     `json:"per_person_min"`
}

type ManualWorkIn struct {
	Description string `json:"description"`
	Minutes     uint   `json:"minutes"`
}

type UserInfo struct {
	DiscordId    string   `json:"discord_id"`
	Name         string   `json:"name"`
	Handle       string   `json:"handle"`
	AvatarUrl    string   `json:"avatar_url,omitempty"`
	Capabilities []string `json:"capabilities"`
	IsPresent    bool     `json:"is_present"`
	IsAdmin      bool     `json:"is_admin"`
}

type LeaderboardRow struct {
	DiscordId     string  `json:"discord_id"`
	Name          string  `json:"name"`
	WorkedCount   int     `json:"worked_count"`
	WorkedMin     float64 `json:"worked_min"`
	AssignedCount int     `json:"assigned_count"`
}

type UserDetail struct {
	DiscordId     string      `json:"discord_id"`
	Name          string      `json:"name"`
	Handle        string      `json:"handle"`
	Performing    []ChoreView `json:"performing"`
	Performed     []ChoreView `json:"performed"`
	TimeSpentMin  uint        `json:"time_spent_min"`
}

type SuggestedPerson struct {
	DiscordId string `json:"discord_id"`
	Name      string `json:"name"`
	HasSkill  bool   `json:"has_skill"`
	Claimed   bool   `json:"claimed"`
	LoadScore int    `json:"load_score"`
}

type SuggestionsResult struct {
	Top    []string          `json:"top"`
	Ranked []SuggestedPerson `json:"ranked"`
}

type PersonPoolEntry struct {
	DiscordId       string   `json:"discord_id"`
	Name            string   `json:"name"`
	Handle          string   `json:"handle"`
	Capabilities    []string `json:"capabilities"`
	WorkloadMin     float64  `json:"workload_min"`
	NormalizedTotal float64  `json:"normalized_total"`
	PresentTicks    int      `json:"present_ticks"`
}

type ScheduledTaskView struct {
	ID                    uint       `json:"id"`
	Name                  string     `json:"name"`
	Description           string     `json:"description,omitempty"`
	CronExpr              string     `json:"cron_expr"`
	NecessaryWorkers      uint       `json:"necessary_workers"`
	EstimatedTimeMin      uint       `json:"estimated_time_min"`
	AssignmentTimeoutMin  uint       `json:"assignment_timeout_min"`
	NecessaryCapabilities []string   `json:"necessary_capabilities"`
	AssigneeId            string     `json:"assignee_id,omitempty"`
	AssigneeName          string     `json:"assignee_name,omitempty"`
	CreatorId             string     `json:"creator_id"`
	CreatorName           string     `json:"creator_name"`
	Enabled               bool       `json:"enabled"`
	TemplateKey           string     `json:"template_key,omitempty"`
	LastRunAt             *time.Time `json:"last_run_at,omitempty"`
	NextRunAt             *time.Time `json:"next_run_at,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
}

type ScheduledTaskIn struct {
	Name                  string   `json:"name"`
	Description           string   `json:"description,omitempty"`
	CronExpr              string   `json:"cron_expr"`
	NecessaryWorkers      uint     `json:"necessary_workers"`
	EstimatedTimeMin      uint     `json:"estimated_time_min"`
	AssignmentTimeoutMin  uint     `json:"assignment_timeout_min"`
	NecessaryCapabilities []string `json:"necessary_capabilities"`
	AssigneeId            string   `json:"assignee_id,omitempty"`
	CreatorId             string   `json:"creator_id,omitempty"`
	Enabled               *bool    `json:"enabled,omitempty"`
	TemplateKey           string   `json:"template_key,omitempty"`
}

