package shifts

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

type DiscordNotifier interface {
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendEmbed(channelID string, embed *discordgo.MessageEmbed, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

type Scheduler struct {
	service          *ShiftsService
	storage          *storage.Storage
	logger           *slog.Logger
	conf             Config
	discord          DiscordNotifier
	discordChannelID string
	lastFrozenDate   string
	remindedShifts   map[uint]bool
	mu               sync.Mutex
}

func NewScheduler(service *ShiftsService, storage *storage.Storage, logger *slog.Logger, conf Config, discord DiscordNotifier, discordChannelID string) *Scheduler {
	return &Scheduler{
		service:          service,
		storage:          storage,
		logger:           logger,
		conf:             conf,
		discord:          discord,
		discordChannelID: discordChannelID,
		remindedShifts:   make(map[uint]bool),
	}
}

func (s *Scheduler) Run(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	s.logger.Info("Starting shifts daily freeze and reminder scheduler", "timezone", s.conf.Timezone, "freeze_hour", s.conf.FreezeHour)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Shifts scheduler stopped")
			return
		case <-ticker.C:
			s.checkDailyFreeze()
			s.checkShiftReminders()
		}
	}
}

func (s *Scheduler) checkDailyFreeze() {
	loc, err := time.LoadLocation(s.conf.Timezone)
	if err != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)

	// Check if current hour is FreezeHour (e.g., 12:00 noon)
	if now.Hour() == s.conf.FreezeHour {
		tomorrow := now.AddDate(0, 0, 1)
		tomorrowStr := tomorrow.Format("2006-01-02")

		s.mu.Lock()
		alreadyDone := s.lastFrozenDate == tomorrowStr
		if !alreadyDone {
			s.lastFrozenDate = tomorrowStr
		}
		s.mu.Unlock()

		if !alreadyDone {
			active, reason, err := s.service.IsSchedulingActiveForDate(tomorrow)
			if err != nil {
				s.logger.Warn("Error checking if scheduling is active for tomorrow", "date", tomorrowStr, "error", err)
			}
			if !active {
				s.logger.Info("No active event, registrations, or present users for tomorrow; skipping freeze", "date", tomorrowStr)
				return
			}

			s.logger.Info("Freezing tomorrow's schedule at noon", "date", tomorrowStr, "reason", reason)
			shifts, err := s.service.GenerateShiftsForDate(tomorrow, true)
			if err != nil {
				s.logger.Error("Failed to auto-freeze tomorrow's schedule", "error", err)
				return
			}
			if len(shifts) > 0 {
				s.announceFrozenSchedule(tomorrowStr, shifts)
			} else {
				s.logger.Info("No shifts defined or scheduled for date, skipping announcement", "date", tomorrowStr)
			}
		}
	}
}

func (s *Scheduler) announceFrozenSchedule(dateStr string, shifts []storage.Shift) {
	if s.discord == nil || s.discordChannelID == "" || len(shifts) == 0 {
		return
	}

	// Verify there is at least one active assignment before announcing
	activeAssignments := 0
	for _, sh := range shifts {
		for _, a := range sh.Assignments {
			if a.Status != "dropped" && a.Status != "refused" && a.Status != "swapped" {
				activeAssignments++
			}
		}
	}
	if activeAssignments == 0 {
		s.logger.Info("No assignments in frozen schedule, skipping announcement", "date", dateStr)
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🔒 Tomorrow's Duty Schedule is Frozen! (%s)", dateStr),
		Description: "The schedule for tomorrow was balanced and locked at 12:00 noon. Please check your assigned shifts below:",
		Color:       0x00b0f4, // Blue
		Fields:      make([]*discordgo.MessageEmbedField, 0),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Garage Trip Chores • Need to swap? Use /shift_swap or click in the web UI",
		},
	}

	users, _ := s.storage.GetUsers()
	userMap := make(map[string]string)
	for _, u := range users {
		name := u.Name
		if name == "" {
			name = u.Handle
		}
		userMap[u.DiscordId] = name
	}

	for _, sh := range shifts {
		timeRange := fmt.Sprintf("%s – %s", sh.StartTime.Format("15:04"), sh.EndTime.Format("15:04"))
		assigneeNames := make([]string, 0)
		for _, a := range sh.Assignments {
			if a.Status != "dropped" && a.Status != "refused" && a.Status != "swapped" {
				name := userMap[a.UserId]
				if name == "" {
					name = fmt.Sprintf("<@%s>", a.UserId)
				}
				assigneeNames = append(assigneeNames, name)
			}
		}

		assigneeStr := "None"
		if len(assigneeNames) > 0 {
			assigneeStr = fmt.Sprintf("👥 %s", stringsJoin(assigneeNames, ", "))
		}

		docStr := ""
		if sh.DocURL != "" {
			docStr = fmt.Sprintf(" • [📄 Runbook Document](%s)", sh.DocURL)
		}

		fieldVal := fmt.Sprintf("**Time:** %s\n**Duty:** %s%s", timeRange, assigneeStr, docStr)
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("[%s] %s", sh.Category, sh.Name),
			Value:  fieldVal,
			Inline: false,
		})
	}

	_, err := s.discord.ChannelMessageSendEmbed(s.discordChannelID, embed)
	if err != nil {
		s.logger.Error("Failed to announce frozen schedule to Discord", "error", err)
	}
}

func (s *Scheduler) checkShiftReminders() {
	now := time.Now()
	leadTime := s.conf.ReminderLeadTime
	if leadTime <= 0 {
		leadTime = 15 * time.Minute
	}

	upcoming, err := s.storage.GetUpcomingShifts(now)
	if err != nil {
		return
	}

	for _, sh := range upcoming {
		s.mu.Lock()
		reminded := s.remindedShifts[sh.ID]
		s.mu.Unlock()

		if reminded {
			continue
		}

		// Check if start time is within leadTime
		if sh.StartTime.After(now) && sh.StartTime.Sub(now) <= leadTime {
			s.mu.Lock()
			s.remindedShifts[sh.ID] = true
			s.mu.Unlock()

			s.sendShiftReminder(sh)
		}
	}
}

func (s *Scheduler) sendShiftReminder(sh storage.Shift) {
	if s.discord == nil || s.discordChannelID == "" {
		return
	}

	mentions := make([]string, 0)
	for _, a := range sh.Assignments {
		if a.Status != "dropped" && a.Status != "refused" && a.Status != "swapped" {
			mentions = append(mentions, fmt.Sprintf("<@%s>", a.UserId))
		}
	}

	if len(mentions) == 0 {
		return
	}

	content := fmt.Sprintf("⏰ **Upcoming Shift Reminder!** %s you are on duty for **%s** starting at **%s**!",
		stringsJoin(mentions, " "), sh.Name, sh.StartTime.Format("15:04"))

	if sh.DocURL != "" {
		content += fmt.Sprintf("\n📄 Please review your shift instructions: %s", sh.DocURL)
	}

	_, err := s.discord.ChannelMessageSend(s.discordChannelID, content)
	if err != nil {
		s.logger.Error("Failed to send shift reminder to Discord", "shift_id", sh.ID, "error", err)
	}
}

func stringsJoin(items []string, sep string) string {
	res := ""
	for i, item := range items {
		if i > 0 {
			res += sep
		}
		res += item
	}
	return res
}
