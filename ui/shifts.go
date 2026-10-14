package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gdg-garage/garage-trip-chores/shifts"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

const (
	ShiftCantAttendClick = "shift_cant_attend" + ButtonClickSuffix
	ShiftSwapAcceptClick = "shift_swap_accept" + ButtonClickSuffix
	ShiftCompleteClick   = "shift_complete" + ButtonClickSuffix
)

func (ui *Ui) SetShifts(s *shifts.ShiftsService) {
	ui.shifts = s
}

func (ui *Ui) isUserOrk(userID string) bool {
	if ui.discord == nil || ui.storage.GetDiscordGuildId() == "" {
		return false
	}
	member, err := ui.discord.GuildMember(ui.storage.GetDiscordGuildId(), userID)
	if err != nil || member == nil {
		return false
	}
	guildRoles, err := ui.discord.GuildRoles(ui.storage.GetDiscordGuildId())
	if err != nil {
		return false
	}
	roleMap := make(map[string]string)
	for _, r := range guildRoles {
		roleMap[r.ID] = r.Name
	}
	for _, rid := range member.Roles {
		rname := roleMap[rid]
		if rname == "g::t::orgs" || rname == "ork" || rname == "chores::admin" {
			return true
		}
	}
	return false
}

func (ui *Ui) shiftsCommand(i *discordgo.InteractionCreate) {
	dateParam := "today"
	options := i.ApplicationCommandData().Options
	for _, opt := range options {
		if opt.Name == "date" {
			dateParam = opt.StringValue()
		}
	}

	targetDate := time.Now()
	if dateParam == "tomorrow" {
		targetDate = targetDate.AddDate(0, 0, 1)
	} else if dateParam != "today" {
		if parsed, err := time.Parse("2006-01-02", dateParam); err == nil {
			targetDate = parsed
		}
	}
	dateStr := targetDate.Format("2006-01-02")

	shiftsList, err := ui.storage.GetShifts(dateStr)
	if err != nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Error fetching shifts: "+err.Error()))
		return
	}

	if len(shiftsList) == 0 {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("No duty shifts scheduled for %s.", dateStr)))
		return
	}

	users, _ := ui.storage.GetUsers()
	userMap := make(map[string]string)
	for _, u := range users {
		name := u.Name
		if name == "" {
			name = u.Handle
		}
		userMap[u.DiscordId] = name
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🛡️ Duty Roster for %s", dateStr),
		Description: "Here are the on-call shifts scheduled for this day:",
		Color:       0x50fa7b, // Green
		Fields:      make([]*discordgo.MessageEmbedField, 0),
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Garage Trip Chores • Check runbooks for duties • 10% standby time credited",
		},
	}

	for _, sh := range shiftsList {
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
			assigneeStr = strings.Join(assigneeNames, ", ")
		}

		docStr := ""
		if sh.DocURL != "" {
			docStr = fmt.Sprintf("\n📄 [Runbook Document](%s)", sh.DocURL)
		}

		fieldVal := fmt.Sprintf("**Time:** %s\n**On Duty:** %s\n**Standby Credit:** +%.0f min%s",
			timeRange, assigneeStr, sh.StandbyCreditedMin(), docStr)

		statusIcon := "⏳"
		if sh.Frozen {
			statusIcon = "🔒"
		}
		if sh.Status == "completed" {
			statusIcon = "✅"
		}

		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s [%s] %s", statusIcon, sh.Category, sh.Name),
			Value:  fieldVal,
			Inline: false,
		})
	}

	_ = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func (ui *Ui) shiftSwapCommand(i *discordgo.InteractionCreate) {
	userId := getInteractionUserId(i)
	var targetUser *discordgo.User
	options := i.ApplicationCommandData().Options
	for _, opt := range options {
		if opt.Name == "user" {
			targetUser = opt.UserValue(ui.discord)
		}
	}

	if targetUser == nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Please specify an attendee to swap with: `/shift_swap @user`."))
		return
	}

	// Find the user's upcoming shifts
	upcoming, err := ui.storage.GetUpcomingShifts(time.Now())
	if err != nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Error fetching shifts: "+err.Error()))
		return
	}

	var myAssignment *storage.ShiftAssignment
	for _, sh := range upcoming {
		for _, a := range sh.Assignments {
			if a.UserId == userId && a.Status == "assigned" {
				myAssignment = &a
				break
			}
		}
		if myAssignment != nil {
			break
		}
	}

	if myAssignment == nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("You don't have any upcoming shifts to swap."))
		return
	}

	if ui.shifts == nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Shifts service is unavailable."))
		return
	}

	swapReq, err := ui.shifts.RequestSwap(myAssignment.ID, targetUser.ID)
	if err != nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Failed to create swap request: "+err.Error()))
		return
	}

	// Post announcement with Accept button for target user
	content := fmt.Sprintf("🔄 <@%s>, <@%s> has requested to swap shifts with you!", targetUser.ID, userId)
	buttonID := fmt.Sprintf("%s%d", ShiftSwapAcceptClick, swapReq.ID)

	_ = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "Accept Shift Swap",
							Style:    discordgo.SuccessButton,
							CustomID: buttonID,
						},
					},
				},
			},
		},
	})
}

func (ui *Ui) shiftFreezeCommand(i *discordgo.InteractionCreate) {
	userId := getInteractionUserId(i)
	if !ui.isUserOrk(userId) {
		_ = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "⛔ Only attendees with the configured **g::t::orgs** role can freeze duty schedules.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	tomorrow := time.Now().AddDate(0, 0, 1)
	dateStr := tomorrow.Format("2006-01-02")

	if ui.shifts == nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Shifts service is unavailable."))
		return
	}

	shiftsList, err := ui.shifts.GenerateShiftsForDate(tomorrow, true)
	if err != nil {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse("Failed to generate and freeze schedule: "+err.Error()))
		return
	}

	if len(shiftsList) == 0 {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse(
			fmt.Sprintf("ℹ️ No shifts are defined or scheduled for tomorrow (%s).", dateStr)))
		return
	}

	activeCount := 0
	for _, sh := range shiftsList {
		for _, a := range sh.Assignments {
			if a.Status != "dropped" && a.Status != "refused" && a.Status != "swapped" {
				activeCount++
			}
		}
	}

	if activeCount == 0 {
		_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse(
			fmt.Sprintf("ℹ️ Tomorrow (%s) has %d shift slots, but no workers were assigned (no eligible or present users).", dateStr, len(shiftsList))))
		return
	}

	_ = ui.discord.InteractionRespond(i.Interaction, simpleInteractionResponse(
		fmt.Sprintf("🔒 Tomorrow's duty schedule (%s) has been generated and **frozen**! (%d shifts, %d assignments)", dateStr, len(shiftsList), activeCount)))
}

func (ui *Ui) handleShiftSwapAccept(customID string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userId := getInteractionUserId(i)
	idStr := strings.TrimPrefix(customID, ShiftSwapAcceptClick)
	swapID, err := strconv.Atoi(idStr)
	if err != nil {
		return
	}

	if ui.shifts == nil {
		_ = s.InteractionRespond(i.Interaction, simpleInteractionResponse("Shifts service is unavailable."))
		return
	}

	err = ui.shifts.AcceptSwap(uint(swapID), userId)
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, simpleInteractionResponse("Could not accept swap: "+err.Error()))
		return
	}

	_ = s.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("✅ Shift swap accepted! <@%s> has taken over the shift.", userId)))
}

func (ui *Ui) handleShiftCantAttend(customID string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	userId := getInteractionUserId(i)
	idStr := strings.TrimPrefix(customID, ShiftCantAttendClick)
	assignID, err := strconv.Atoi(idStr)
	if err != nil {
		return
	}

	if ui.shifts == nil {
		_ = s.InteractionRespond(i.Interaction, simpleInteractionResponse("Shifts service is unavailable."))
		return
	}

	replacement, err := ui.shifts.CanAttendShift(uint(assignID), "Reported via Discord")
	if err != nil {
		_ = s.InteractionRespond(i.Interaction, simpleInteractionResponse("Shift dropped, but no immediate replacement found: "+err.Error()))
		return
	}

	_ = s.InteractionRespond(i.Interaction, simpleInteractionResponse(
		fmt.Sprintf("⚠️ Shift dropped by <@%s>. Successfully reassigned to replacement <@%s>!", userId, replacement.UserId)))
}
