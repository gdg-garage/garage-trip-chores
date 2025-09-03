package ui

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"gorm.io/gorm"
)

type Colors struct {
	OrangeColor int
	GreenColor  int
	RedColor    int
}

type Ui struct {
	storage *storage.Storage
	logger  *slog.Logger
	chores  *chores.ChoresLogic
	discord *discordgo.Session
	conf    Config
	colors  Colors
}

const (
	buttonClickSuffix    = "_button_click:"
	ackButtonClick       = "ack" + buttonClickSuffix
	cancelButtonClick    = "cancel" + buttonClickSuffix
	deleteButtonClick    = "delete" + buttonClickSuffix
	doneButtonClick      = "done" + buttonClickSuffix
	editButtonClick      = "edit" + buttonClickSuffix
	rejectButtonClick    = "reject" + buttonClickSuffix
	scheduleButtonClick  = "schedule" + buttonClickSuffix
	helpedButtonClick    = "helped" + buttonClickSuffix
	reportTimeSpentClick = "report_time_spent" + buttonClickSuffix

	ModalSubmitSuffix    = "_modal_submit:"
	reportTimeSpentModal = "report_time_spent" + ModalSubmitSuffix
)

func simpleInteractionResponse(content string) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
			Components: []discordgo.MessageComponent{
				discordgo.TextDisplay{
					Content: content,
				},
			},
		},
	}
}

func simpleContainerizedInteractionResponse(content string, color *int) *discordgo.InteractionResponse {
	return &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
			Components: []discordgo.MessageComponent{
				discordgo.Container{
					AccentColor: color,
					Components: []discordgo.MessageComponent{
						discordgo.TextDisplay{
							Content: content,
						},
					},
				},
			},
		},
	}
}

func (ui *Ui) errorInteractionResponse(content string) *discordgo.InteractionResponse {
	return simpleContainerizedInteractionResponse(content, &ui.colors.RedColor)
}

func getChoreIdFromButton(customID string) (uint, error) {
	// Extract the chore ID from the custom ID.
	// The custom ID format is "button_id:<chore_id>"
	parts := strings.Split(customID, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid custom ID format: %s", customID)
	}

	var choreId uint
	_, err := fmt.Sscanf(parts[1], "%d", &choreId)
	if err != nil {
		return 0, fmt.Errorf("failed to parse chore ID from custom ID: %w", err)
	}

	return choreId, nil
}

func (ui *Ui) sendDM(discordId string, message *discordgo.MessageSend) error {
	dmChannel, err := ui.discord.UserChannelCreate(discordId)
	if err != nil {
		return fmt.Errorf("failed to create DM channel: %w", err)
	}

	_, err = ui.discord.ChannelMessageSendComplex(dmChannel.ID, message)
	if err != nil {
		return fmt.Errorf("failed to send DM: %w", err)
	}
	return nil
}

func (ui *Ui) getChoreMessageUrl(c storage.Chore) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", ui.storage.GetDiscordGuildId(), ui.conf.DiscordChannelId, c.MessageId)
}

func (ui *Ui) scheduleChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to schedule chore."
	choreId, err := getChoreIdFromButton(buttonId)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	c, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	users, err := ui.storage.GetPresentUsers()
	if err != nil {
		ui.logger.Error("Error getting present users", "error", err)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	} else {
		for _, user := range users {
			ui.logger.Debug("Present user", "user", user.Handle, "capabilities", user.Capabilities)
		}
	}
	ass, err := ui.chores.AssignChoresToUsers(users, c)
	if err != nil {
		ui.logger.Error("Error assigning chores to users", "error", err)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	} else {
		ui.logger.Debug("Chores assigned to users", "cnt", len(ass))
		for _, a := range ass {
			ui.logger.Debug("Chore assigned to user", "assignment", a)
		}
	}

	embeds := []*discordgo.MessageEmbed{}

	choreMd := ui.generateChoreMd(c)
	choreEmbed := discordgo.MessageEmbed{
		Type:        discordgo.EmbedTypeRich,
		Description: choreMd,
	}
	embeds = append(embeds, &choreEmbed)

	assignmentsEmbed := ui.generateAssignmentEmbed(ass, "Assignments", ui.colors.OrangeColor)
	if assignmentsEmbed != nil {
		embeds = append(embeds, assignmentsEmbed)
	}

	// Send a public message to the channel announcing the scheduled chore
	m, err := s.ChannelMessageSendComplex(ui.conf.DiscordChannelId, &discordgo.MessageSend{
		Content: c.Name,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					&discordgo.Button{
						Style:    discordgo.PrimaryButton,
						Label:    "Ack",
						CustomID: "ack_button_click:" + fmt.Sprint(c.ID),
					},
					&discordgo.Button{
						Style:    discordgo.SecondaryButton,
						Label:    "Reject",
						CustomID: "reject_button_click:" + fmt.Sprint(c.ID),
					},
				},
			},
		},
		Embeds: embeds,
	})
	if err != nil {
		ui.logger.Error("failed to send public chore message", "error", err, "chore_id", c.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	c.MessageId = m.ID
	_, err = ui.storage.SaveChore(c)
	if err != nil {
		ui.logger.Error("failed to save chore with message ID", "error", err, "chore_id", c.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ui.logger.Info("Chore scheduled and published", "chore_id", c.ID, "message_id", m.ID)

	messageUrl := ui.getChoreMessageUrl(c)
	err = ui.sendDM(c.CreatorId, &discordgo.MessageSend{
		Content: fmt.Sprintf("Your chore `%s` (id: `%d`) was scheduled and published in <#%s>.\n%s", c.Name, c.ID, ui.conf.DiscordChannelId, messageUrl),
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					&discordgo.Button{
						Style:    discordgo.SuccessButton,
						Label:    "Done!",
						CustomID: doneButtonClick + fmt.Sprint(c.ID),
					},
					&discordgo.Button{
						Style:    discordgo.DangerButton,
						Label:    "Cancel",
						CustomID: cancelButtonClick + fmt.Sprint(c.ID),
					},
				},
			},
		},
	})
	if err != nil {
		ui.logger.Warn("failed to send DM to creator", "error", err, "chore_id", c.ID, "creator_id", c.CreatorId)
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` was scheduled and published.", choreId), &ui.colors.GreenColor)
	r.Data.Components = append(r.Data.Components, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			&discordgo.Button{
				Style:    discordgo.SuccessButton,
				Label:    "Done!",
				CustomID: "done_button_click:" + fmt.Sprint(choreId),
			},
			&discordgo.Button{
				Style:    discordgo.DangerButton,
				Label:    "Cancel",
				CustomID: "cancel_button_click:" + fmt.Sprint(choreId),
			},
		},
	})

	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) generateWorkLogEmbed(wl []storage.WorkLog) *discordgo.MessageEmbed {
	if len(wl) == 0 {
		return nil
	}
	worklogMd := ""

	for _, w := range wl {
		worklogMd += fmt.Sprintf("* <@%s>: %d min\n", w.UserId, w.TimeSpentMin)
	}

	worklogEmbed := discordgo.MessageEmbed{
		Type:        discordgo.EmbedTypeRich,
		Title:       "Workers",
		Description: worklogMd,
		Color:       ui.colors.GreenColor,
	}
	return &worklogEmbed
}

func (ui *Ui) generateAssignmentEmbed(ass []storage.ChoreAssignment, title string, color int) *discordgo.MessageEmbed {
	if len(ass) == 0 {
		return nil
	}
	assignmentsMd := ""

	for _, a := range ass {
		assignmentsMd += fmt.Sprintf("<@%s> ", a.UserId)
	}

	assignmentsEmbed := discordgo.MessageEmbed{
		Type:        discordgo.EmbedTypeRich,
		Title:       title,
		Description: assignmentsMd,
		Color:       color,
	}
	return &assignmentsEmbed
}

func (ui *Ui) editChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to edit chore."
	choreId, err := getChoreIdFromButton(buttonId)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	_, err = ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		return
	}
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	t := time.Now()
	chore.Cancelled = &t // Set the cancelled time to now
	_, err = ui.storage.SaveChore(chore)
	if err != nil {
		ui.logger.Error("failed to save chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` has been removed.", choreId), &ui.colors.RedColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) cancelChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to remove chore."
	choreId, err := getChoreIdFromButton(buttonId)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	if chore.Cancelled != nil {
		r := simpleContainerizedInteractionResponse(fmt.Sprintf("Chore `id: %d` has been cancelled.", choreId), &ui.colors.RedColor)
		s.InteractionRespond(i.Interaction, r)
		return
	}
	if chore.Completed != nil {
		r := simpleContainerizedInteractionResponse(fmt.Sprintf("Chore `id: %d` has been completed and cannot be cancelled.", choreId), &ui.colors.RedColor)
		s.InteractionRespond(i.Interaction, r)
		return
	}
	t := time.Now()
	chore.Cancelled = &t // Set the cancelled time to now
	_, err = ui.storage.SaveChore(chore)
	if err != nil {
		ui.logger.Error("failed to save chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	err = ui.storage.RemoveStorageAssignments(choreId)
	if err != nil {
		ui.logger.Error("failed to remove chore assignments", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` has been removed.", choreId), &ui.colors.RedColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)

	err = ui.updateChoreMessage(chore)
	if err != nil {
		ui.logger.Error("failed to update chore message", "error", err, "chore_id", chore.ID)
	}
}

func (ui *Ui) rejectChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to reject chore."
	choreId, err := getChoreIdFromButton(buttonId)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	c, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ui.logger.Debug("Rejecting chore", "chore_id", c.ID, "user_id", i.Member)
	ass, err := ui.storage.GetChoreAssignment(c.ID, i.Member.User.ID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ui.logger.Error("Chore assignment not found", "error", err, "chore_id", c.ID, "user_id", i.Member.User.ID)
			s.InteractionRespond(i.Interaction, ui.errorInteractionResponse("Chore cannot be rejected, you are not assigned to it."))
			return
		}
		ui.logger.Error("failed to get chore assignment", "error", err, "chore_id", c.ID, "user_id", i.Member.User.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ass.Refuse()
	_, err = ui.storage.SaveChoreAssignment(ass)
	if err != nil {
		ui.logger.Error("failed to save chore assignment", "error", err, "chore_id", c.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	s.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("Chore `%d` rejected\n\n*... Dissapointing*", c.ID)))

	users, err := ui.storage.GetPresentUsers()
	if err != nil {
		ui.logger.Error("Error getting present users", "error", err)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ui.chores.AssignChoresToUsers(users, c)
	ui.updateChoreMessage(c)
}

func (ui *Ui) ackChore(customID string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to acknowledge chore."
	choreId, err := getChoreIdFromButton(customID)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", customID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	var ass storage.ChoreAssignment

	c, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	ass, err = ui.storage.GetChoreAssignment(choreId, i.Member.User.ID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new assignment
			ass, err = ui.storage.AssignChore(c, i.Member.User.ID)
			if err != nil {
				ui.logger.Error("failed to assign chore", "error", err, "chore_id", choreId, "user_id", i.Member.User.ID)
				s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
				return
			}
			ass.Volunteered = true
			_, err = ui.storage.SaveChoreAssignment(ass)
			if err != nil {
				ui.logger.Error("failed to save chore assignment", "error", err, "chore_id", choreId)
				s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
				return
			}
		} else {
			ui.logger.Error("failed to get chore assignment", "error", err, "chore_id", choreId, "user_id", i.Member.User.ID)
			s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
			return
		}
	}

	ass.Ack()

	_, err = ui.storage.SaveChoreAssignment(ass)
	if err != nil {
		ui.logger.Error("failed to save chore assignment", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	s.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("Chore `%s` (id: `%d`) acknowledged.", c.Name, c.ID)))

	ui.updateChoreMessage(c)
}

func (ui *Ui) updateChoreMessage(chore storage.Chore) error {
	if chore.MessageId == "" {
		ui.logger.Info("Chore message ID is empty, skipping update", "chore_id", chore.ID)
		return nil
	}

	embeds := []*discordgo.MessageEmbed{}

	choreMd := ui.generateChoreMd(chore)
	choreEmbed := discordgo.MessageEmbed{
		Type:        discordgo.EmbedTypeRich,
		Description: choreMd,
	}
	embeds = append(embeds, &choreEmbed)

	worklogs, err := ui.storage.GetWorkLogsForChore(chore.ID)
	if err != nil {
		ui.logger.Error("failed to get work logs for chore", "error", err, "chore_id", chore.ID)
		return err
	}

	worklogEmbed := ui.generateWorkLogEmbed(worklogs)
	if worklogEmbed != nil {
		embeds = append(embeds, worklogEmbed)
	}

	assignmentsAll, err := ui.storage.GetChoreAssignments(chore.ID)
	if err != nil {
		ui.logger.Error("failed to get chore assignments", "error", err, "chore_id", chore.ID)
		return err
	}

	assignments := []storage.ChoreAssignment{}
	timeouted := []storage.ChoreAssignment{}
	acked := []storage.ChoreAssignment{}
	declined := []storage.ChoreAssignment{}

	for _, a := range assignmentsAll {
		if a.Acked != nil {
			acked = append(acked, a)
		} else if a.Refused != nil {
			declined = append(declined, a)
		} else if a.Timeouted != nil {
			timeouted = append(timeouted, a)
		} else {
			assignments = append(assignments, a)
		}
	}
	assignmentsEmbed := ui.generateAssignmentEmbed(assignments, "Assignments", ui.colors.OrangeColor)
	if assignmentsEmbed != nil {
		embeds = append(embeds, assignmentsEmbed)
	}

	timeoutedEmbed := ui.generateAssignmentEmbed(timeouted, "Timeouted", ui.colors.RedColor)
	if timeoutedEmbed != nil {
		embeds = append(embeds, timeoutedEmbed)
	}

	ackedEmbed := ui.generateAssignmentEmbed(acked, "Acknowledged", ui.colors.GreenColor)
	if ackedEmbed != nil {
		embeds = append(embeds, ackedEmbed)
	}

	declinedEmbed := ui.generateAssignmentEmbed(declined, "Declined", ui.colors.RedColor)
	if declinedEmbed != nil {
		embeds = append(embeds, declinedEmbed)
	}

	buttons := []discordgo.MessageComponent{}

	if chore.Completed == nil && chore.Cancelled == nil {
		buttons = append(buttons,
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					&discordgo.Button{
						Style:    discordgo.PrimaryButton,
						Label:    "Ack",
						CustomID: ackButtonClick + fmt.Sprint(chore.ID),
					},
					&discordgo.Button{
						Style:    discordgo.SecondaryButton,
						Label:    "Reject",
						CustomID: rejectButtonClick + fmt.Sprint(chore.ID),
					},
				},
			})
	} else if chore.Completed != nil {
		buttons = append(buttons,
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					&discordgo.Button{
						Style:    discordgo.SuccessButton,
						Label:    "I helped",
						CustomID: helpedButtonClick + fmt.Sprint(chore.ID),
					},
				},
			})
	}

	name := ""
	if chore.Completed != nil {
		name = "✅ " + chore.Name
	} else if chore.Cancelled != nil {
		name = "❌ " + chore.Name
	} else {
		name = chore.Name
	}

	_, err = ui.discord.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			Content:    &name,
			ID:         chore.MessageId,
			Channel:    ui.conf.DiscordChannelId,
			Components: &buttons,
			Embeds:     &embeds,
		},
	)
	if err != nil {
		ui.logger.Error("failed to edit chore message", "error", err, "chore_id", chore.ID, "message_id", chore.MessageId)
		return err
	}

	return nil
}

func (ui *Ui) generateChoreMd(chore storage.Chore) string {
	isCompleted := false
	if chore.Completed != nil {
		isCompleted = true
	}
	isCancelled := false
	if chore.Cancelled != nil {
		isCancelled = true
	}
	name := chore.Name
	if isCompleted {
		name = "✅ " + name
	}
	if isCancelled {
		name = "❌ " + name
	}
	choreDesc := fmt.Sprintf("### Name: `%s`\n"+
		"**Creator**: <@%s>\n"+
		"**ID**: `%d`\n"+
		"**Estimated Time (min)**: `%d`\n"+
		"**Necessary Workers**: `%d`\n"+
		"**Assignment Timeout (min)**: `%d`",
		name, chore.CreatorId, chore.ID, chore.EstimatedTimeMin, chore.NecessaryWorkers,
		chore.AssignmentTimeoutMin)

	if chore.Deadline != nil {
		choreDesc += fmt.Sprintf("\n**Deadline**: %s", chore.Deadline.Format(time.RFC822))
	}
	if isCompleted {
		choreDesc += fmt.Sprintf("\n**Completed**: %s", chore.Completed.Format(time.RFC822))
	}
	if isCancelled {
		choreDesc += fmt.Sprintf("\n**Cancelled**: %s", chore.Cancelled.Format(time.RFC822))
	}

	return choreDesc
}

func (ui *Ui) choreCreate(i *discordgo.InteractionCreate) {
	// Respond to the slash command interaction.
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	defaultDeadline := time.Now().Add(24 * time.Hour) // Default deadline is 24 hours from creation
	chore := storage.Chore{
		Name:                 optionMap["name"].StringValue(),
		NecessaryWorkers:     uint(1),
		EstimatedTimeMin:     uint(10),
		AssignmentTimeoutMin: uint(15),
		Deadline:             &defaultDeadline,
		CreatorId:            i.Member.User.ID, // Discord ID of the user who created the chore
		Created:              time.Now(),       // Timestamp when the chore was created
	}

	for k, v := range optionMap {
		switch k {
		case "estimated_time_min":
			chore.EstimatedTimeMin = uint(v.IntValue())
		case "necessary_workers":
			chore.NecessaryWorkers = uint(v.IntValue())
		case "assignment_timeout_min":
			chore.AssignmentTimeoutMin = uint(v.IntValue())
		case "deadline":
			if v.IntValue() > 0 {
				deadline := time.Now().Add(time.Duration(v.IntValue()) * time.Minute)
				chore.Deadline = &deadline // Set the deadline if provided
			}
		case "capabilities":
			if v.StringValue() != "" {
				chore.SetCapabilities(strings.Split(v.StringValue(), ","))
			}
		}
	}

	chore, err := ui.storage.SaveChore(chore)
	if err != nil {
		ui.logger.Error("failed to save chore", "error", err)
		ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
				Components: []discordgo.MessageComponent{
					discordgo.TextDisplay{
						Content: "Failed to create chore",
					},
				},
			},
		})
		return
	}

	choreDesc := ui.generateChoreMd(chore)

	capabilityOptions := []discordgo.SelectMenuOption{}

	capbilitiesMap := map[string]struct{}{}
	for _, cap := range chore.GetCapabilities() {
		capbilitiesMap[cap] = struct{}{}
	}

	skills, err := ui.storage.GetSkills()
	if err != nil {
		ui.logger.Error("failed to get skills", "error", err)
		skills = []string{} // Fallback to empty skills if there's an error
	}

	for _, r := range skills {
		s := discordgo.SelectMenuOption{
			Label:       r,
			Value:       r,
			Description: fmt.Sprintf("This chore requires the %s skill.", r),
		}
		if _, ok := capbilitiesMap[r]; ok {
			s.Default = true
		}
		capabilityOptions = append(capabilityOptions, s)
	}

	ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			// Content: "Please check the chore",
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2, // Makes the message visible only to the user who invoked the command.

			// 	discordgo.ActionsRow{
			// 		Components: []discordgo.MessageComponent{
			// 			&discordgo.SelectMenu{
			// 				CustomID:    "hello_select_menu",
			// 				Placeholder: "Choose an option",
			// 				MaxValues:   2,
			// 				Options: []discordgo.SelectMenuOption{
			// 					{
			// 						Label:       "Option 1",
			// 						Value:       "option_1",
			// 						Description: "This is the first option.",
			// 						Default:     true, // Default option selected
			// 					},
			// 					{
			// 						Label:       "Option 2",
			// 						Value:       "option_2",
			// 						Description: "This is the second option.",
			// 					},
			// 				},
			// 			},
			// 		},
			// 	},
			// },

			Components: []discordgo.MessageComponent{
				// 					discordgo.Container{
				// 						AccentColor: &green, // Green color
				// 						Components: []discordgo.MessageComponent{
				// 							discordgo.TextDisplay{
				// 								Content: fmt.Sprintf("Chore created by **%s** with ID `%s`", chore.CreatorHandle, i.Interaction.ID),
				// 							},
				// 						},
				// 					},

				&discordgo.TextDisplay{
					Content: "Please check the chore",
				},

				discordgo.Container{
					AccentColor: &ui.colors.OrangeColor,
					Components: []discordgo.MessageComponent{
						&discordgo.TextDisplay{
							Content: choreDesc,
						},
					},
				},

				// discorgo.Embed

				// discordgo.Container{
				// 	Components: []discordgo.MessageComponent{
				// 		discordgo.TextDisplay{
				// 			Content: fmt.Sprintf("Chore created by **%s** with ID `%s`", chore.CreatorHandle, i.Interaction.ID),
				// 		},
				// 	},
				// },

				discordgo.Container{
					Components: []discordgo.MessageComponent{
						&discordgo.TextDisplay{
							Content: "Skills required for this chore:",
						},

						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{

								&discordgo.SelectMenu{
									CustomID:    "skills_select_menu",
									Placeholder: "Required skills for this chore",
									MaxValues:   len(capabilityOptions), // Allow selecting all
									Options:     capabilityOptions,
								},
							},
						},
					},
				},

				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.SuccessButton,
							Label:    "Schedule",
							CustomID: scheduleButtonClick + fmt.Sprint(chore.ID),
						},
						&discordgo.Button{
							Style:    discordgo.SecondaryButton,
							Label:    "Edit",
							CustomID: editButtonClick + fmt.Sprint(chore.ID),
						},
						&discordgo.Button{
							Style:    discordgo.DangerButton,
							Label:    "Delete",
							CustomID: deleteButtonClick + fmt.Sprint(chore.ID),
						},
					},
				},
			},
		},
	})

	// Respond to the slash command interaction.
	// s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
	// 	Content: "Hell yeah!",
	// 	Flags:   discordgo.MessageFlagsEphemeral, // Makes the message visible only to the user who invoked the command.
	// })
}

func NewUi(storage *storage.Storage, logger *slog.Logger, chores *chores.ChoresLogic, discord *discordgo.Session, conf Config) *Ui {
	return &Ui{
		storage: storage,
		logger:  logger,
		chores:  chores,
		discord: discord,
		conf:    conf,
		colors: Colors{
			OrangeColor: 0xFFA500,
			GreenColor:  0x00FF00,
			RedColor:    0xFF0000,
		},
	}
}

func (ui *Ui) Commands(ctx context.Context, wg *sync.WaitGroup) error {
	defer wg.Done()
	// 2. Register a handler for incoming interactions (like slash commands).
	ui.discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Interaction.Type == discordgo.InteractionApplicationCommand { // Ensure the interaction type is set correctly.
			if i.Interaction.ChannelID != ui.conf.DiscordChannelId {
				// If the interaction is not in a channel, we can't respond.
				s.InteractionRespond(i.Interaction, simpleInteractionResponse("This command can only be used in <#"+ui.conf.DiscordChannelId+"> channel."))
				return
			}
			// Check if the interaction is an ApplicationCommand (a slash command).
			switch i.ApplicationCommandData().Name {
			case "chore_create":
				ui.choreCreate(i) // Call the appropriate handler function.
			case "chores":
				ui.choresList(i)
			case "chores_open":
				ui.choresOpen(i)
			case "chores_completed":
				ui.choresCompleted(i)
			case "stats":
				ui.stats(i)
			}
		}

		if i.Type == discordgo.InteractionMessageComponent {
			data := i.MessageComponentData()
			switch {
			case strings.HasPrefix(data.CustomID, deleteButtonClick) || strings.HasPrefix(data.CustomID, cancelButtonClick):
				ui.cancelChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, editButtonClick):
				ui.editChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, scheduleButtonClick):
				ui.scheduleChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, rejectButtonClick):
				ui.rejectChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, ackButtonClick):
				ui.ackChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, doneButtonClick):
				ui.doneChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, helpedButtonClick):
				ui.helpedChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, reportTimeSpentClick):
				ui.reportTimeSpentButtonClick(data.CustomID, s, i)
			}
		}

		if i.Type == discordgo.InteractionModalSubmit {
			data := i.ModalSubmitData()
			switch {
			case strings.HasPrefix(data.CustomID, reportTimeSpentModal):
				ui.reportTimeSpent(s, i)
			}
		}
	})

	// 3. Set the intent to receive GuildMessages and Guilds.
	// This is necessary for the bot to function correctly, especially for commands.
	ui.discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds

	// 4. Open the WebSocket connection to Discord.
	// err := s.discord.Open()
	// if err != nil {
	// 	log.Fatalf("Error opening connection: %v", err)
	// 	return nil
	// }

	skillsChoice := []*discordgo.ApplicationCommandOptionChoice{}

	skills, err := ui.storage.GetSkills()
	if err != nil {
		ui.logger.Error("failed to get skills", "error", err)
		skills = []string{} // Fallback to empty skills if there's an error
	}
	for _, skill := range skills {
		skillsChoice = append(skillsChoice, &discordgo.ApplicationCommandOptionChoice{
			Name:  skill,
			Value: skill,
		})
	}

	// Command definitions for our slash commands.
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "chore_create",
			Description: "Creates a new chore.",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "The chore description.",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "necessary_workers",
					Description: "The number of workers required to complete the chore. [1]",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "estimated_time_min",
					Description: "The estimated time to complete the chore in minutes. [10]",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "capabilities",
					Description: "The capabilities (skills) required to complete the chore.",
					Required:    false,
					Choices:     skillsChoice,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "deadline",
					Description: "The deadline for the chore in minutes from now. [24h]",
					Required:    false,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{
							Name:  "5 minutes",
							Value: 5,
						},
						{
							Name:  "10 minutes",
							Value: 10,
						},
						{
							Name:  "15 minutes",
							Value: 15,
						},
						{
							Name:  "30 minutes",
							Value: 30,
						},
						{
							Name:  "1 hour",
							Value: 60,
						},
						{
							Name:  "2 hours",
							Value: 60 * 2,
						},
						{
							Name:  "4 hours",
							Value: 60 * 4,
						},
						{
							Name:  "8 hours",
							Value: 60 * 8,
						},
						{
							Name:  "12 hours",
							Value: 60 * 12,
						},
						{
							Name:  "24 hours",
							Value: 60 * 24,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "assignment_timeout_min",
					Description: "The time in minutes after which the chore will be unassigned if not acked (0 to disable). [15]",
					Required:    false,
				},

				// {
				// 	Type:        discordgo.ApplicationCommandOptionBoolean,
				// 	Name:        "is_admin",
				// 	Description: "Is the user an admin?",
				// 	Required:    true,
				// },
			},
		},
		{
			Name:        "chores",
			Description: "Lists your chores.",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "chores_open",
			Description: "Lists unfinished chores.",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "chores_completed",
			Description: "Lists completed chores.",
			Type:        discordgo.ChatApplicationCommand,
		},
		{
			Name:        "stats",
			Description: "Display chores stats.",
			Type:        discordgo.ChatApplicationCommand,
		},
	}

	// 5. Register the slash commands globally.
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := ui.discord.ApplicationCommandCreate(ui.discord.State.User.ID, ui.storage.GetDiscordGuildId(), v)
		if err != nil {
			ui.logger.Error("Cannot create command", "name", v.Name, "error", err)
		}
		registeredCommands[i] = cmd
	}
	ui.logger.Info("Commands registered successfully!")

	// 6. Keep the bot running until an interrupt signal is received.
	// fmt.Println("Bot is now running. Press CTRL-C to exit.")
	// sc := make(chan os.Signal, 1)
	// signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	// <-sc // Block until a signal is received.

	<-ctx.Done()

	// 7. Cleanly close the Discord session.
	ui.logger.Info("Shutting down Discord session...")
	ui.discord.Close()

	// 8. Unregister the commands when the bot shuts down.
	// This is good practice to avoid stale commands.
	ui.logger.Info("Unregistering commands...")
	for _, v := range registeredCommands {
		err := ui.discord.ApplicationCommandDelete(ui.discord.State.User.ID, ui.storage.GetDiscordGuildId(), v.ID)
		if err != nil {
			ui.logger.Error("Cannot delete command", "name", v.Name, "error", err)
		}
	}
	ui.logger.Info("Commands unregistered.")
	return nil
}

func (ui *Ui) reportTimeSpentButtonClick(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to report time spent."

	choreId, err := getChoreIdFromButton(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	err = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: reportTimeSpentModal + fmt.Sprint(choreId),
			Title:    fmt.Sprintf("Report time spent on chore %d", choreId),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID:    "time_spent_min",
							Label:       "Time Spent (minutes)",
							Style:       discordgo.TextInputShort,
							MinLength:   1,
							MaxLength:   4,
							Placeholder: "Enter time spent on chore",
							Required:    true,
						},
					},
				},
			},
		},
	})
	if err != nil {
		ui.logger.Error("failed to send modal", "error", err)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
}

func (ui *Ui) reportTimeSpent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to report time spent."
	ui.logger.Debug("Reporting time spent", "custom_id", i.ModalSubmitData().CustomID)
	data := i.Interaction.ModalSubmitData()

	choreId, err := getChoreIdFromButton(data.CustomID)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from modal", "error", err, "custom_id", data.CustomID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ui.logger.Debug("Reporting time spent", "chore_id", choreId)

	timeSpentStr := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	timeSpent, err := strconv.Atoi(timeSpentStr)
	if err != nil {
		ui.logger.Error("failed to parse time spent", "error", err, "input", timeSpentStr)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ui.logger.Debug("Reporting time spent", "chore_id", choreId, "time_spent", timeSpent)

	userId := i.Interaction.User.ID

	ui.logger.Debug("Reporting time spent", "chore_id", choreId, "user_id", userId, "time_spent", timeSpent)
	wl, err := ui.storage.GetWorkLogForChoreAndUser(choreId, userId)
	if err != nil {
		ui.logger.Error("failed to get work log", "error", err, "chore_id", choreId, "user_id", userId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	wl.TimeSpentMin = uint(timeSpent)
	_, err = ui.storage.SaveWorkLog(wl)
	if err != nil {
		ui.logger.Error("failed to save work log", "error", err, "chore_id", choreId, "user_id", userId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	r := simpleContainerizedInteractionResponse(fmt.Sprintf("Updated time spent on chore `id: %d` to `%d` min.", choreId, timeSpent), &ui.colors.GreenColor)
	s.InteractionRespond(i.Interaction, r)
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ui.updateChoreMessage(chore)
}

func (ui *Ui) stats(i *discordgo.InteractionCreate) {
	failedText := "Failed to get stats."
	embeds := []*discordgo.MessageEmbed{}

	type UserStats struct {
		workedCount     float64
		WorkedMin       float64
		AssignedMin     float64
		assignedCount   float64
		TotalMin        float64
		TotalCount      float64
		PresentTicks    int
		NormalizedTotal float64
	}

	usersStats := map[string]UserStats{}

	userStats, err := ui.storage.GetUserStats()
	if err != nil {
		ui.logger.Error("failed to get user stats", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	for k, v := range userStats {
		_, ok := usersStats[k]
		if !ok {
			usersStats[k] = UserStats{}
		}
		s := usersStats[k]
		s.workedCount = v.Count
		s.WorkedMin = v.TotalMin
		usersStats[k] = s
	}

	assignedStats, err := ui.storage.GetAssignedStats()
	if err != nil {
		ui.logger.Error("failed to get assigned stats", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	for k, v := range assignedStats {
		_, ok := usersStats[k]
		if !ok {
			usersStats[k] = UserStats{}
		}
		s := usersStats[k]
		s.AssignedMin = v.TotalMin
		s.assignedCount = v.Count
		usersStats[k] = s
	}

	totalStats, err := ui.storage.GetTotalChoreStats()
	if err != nil {
		ui.logger.Error("failed to get total chore stats", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	for k, v := range totalStats {
		_, ok := usersStats[k]
		if !ok {
			usersStats[k] = UserStats{}
		}
		s := usersStats[k]
		s.TotalMin = v.TotalMin
		s.TotalCount = v.Count
		usersStats[k] = s
	}

	usersPresenceCounts, err := ui.storage.GetUsersPresenceCounts()
	if err != nil {
		ui.logger.Error("failed to get users presence counts", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	for k, v := range usersPresenceCounts {
		_, ok := usersStats[k]
		if !ok {
			usersStats[k] = UserStats{}
		}
		s := usersStats[k]
		s.PresentTicks = v
		usersStats[k] = s
	}

	normalizedStats, err := ui.storage.GetTotalNormalizedChoreStats()
	if err != nil {
		ui.logger.Error("failed to get normalized chore stats", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	for k, v := range normalizedStats {
		_, ok := usersStats[k]
		if !ok {
			usersStats[k] = UserStats{}
		}
		s := usersStats[k]
		s.NormalizedTotal = v.TotalMin
		usersStats[k] = s
	}

	// Convert map to slice for sorting
	type kv struct {
		Key   string
		Value UserStats
	}
	var ss []kv
	for k, v := range usersStats {
		ss = append(ss, kv{k, v})
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value.NormalizedTotal > ss[j].Value.NormalizedTotal
	})

	statsMd := `
* WorkedCnt
* WorkedMin
* AssignedCnt
* AssignedMin
* TotalCnt
* TotalMin
* PresenceTicks
* NormalizedTotal
`
	statsMd += "```WC\tWM\tAC\tAM\tTC\tTM\tPT\tNT```\n"
	for _, v := range ss {
		k := v.Key
		c := v.Value
		statsMd += fmt.Sprintf("<@%s>\n", k)
		statsMd += fmt.Sprintf("```%0.f\t%0.f\t%0.f\t%0.f\t%0.f\t%0.f\t%d\t%.2f```\n",
			c.workedCount, c.WorkedMin, c.assignedCount, c.AssignedMin, c.TotalCount, c.TotalMin, c.PresentTicks, c.NormalizedTotal)
	}
	embed := discordgo.MessageEmbed{
		Title:       "User stats:",
		Description: statsMd,
		Color:       ui.colors.GreenColor,
	}
	embeds = append(embeds, &embed)

	r := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Here are user stats:",
			Embeds:  embeds,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}
	ui.discord.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) choresCompleted(i *discordgo.InteractionCreate) {
	limit := 15
	failedText := "Failed to get completed chores."
	embeds := []*discordgo.MessageEmbed{}

	completedChores, err := ui.storage.GetCompletedChores()
	if err != nil {
		ui.logger.Error("failed to get completed chores", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	completedMd := ""
	for _, c := range completedChores[:int(math.Min(float64(len(completedChores)), float64(limit)))] {
		completedMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.getChoreMessageUrl(c))
	}
	embed := discordgo.MessageEmbed{
		Title:       "Completed chores",
		Description: completedMd,
		Color:       ui.colors.GreenColor,
	}
	embeds = append(embeds, &embed)

	r := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Here are recent completed chores:",
			Embeds:  embeds,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}
	ui.discord.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) choresOpen(i *discordgo.InteractionCreate) {
	failedText := "Failed to get open chores."
	embeds := []*discordgo.MessageEmbed{}

	openChores, err := ui.storage.GetUnfinishedChores()
	if err != nil {
		ui.logger.Error("failed to get open chores", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	openMd := ""
	for _, c := range openChores {
		openMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.getChoreMessageUrl(c))
	}
	embed := discordgo.MessageEmbed{
		Title:       "Open chores",
		Description: openMd,
		Color:       ui.colors.GreenColor,
	}
	embeds = append(embeds, &embed)

	r := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Here are all open chores:",
			Embeds:  embeds,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}
	ui.discord.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) choresList(i *discordgo.InteractionCreate) {
	userId := i.Interaction.Member.User.ID
	failedText := "Failed to get chore assignments."
	embeds := []*discordgo.MessageEmbed{}

	// get assigned chores for the user
	assignedChores, err := ui.storage.GetAssignedChoresForUser(userId)
	if err != nil {
		ui.logger.Error("failed to get assigned chores", "error", err, "user_id", userId)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	assignedMd := ""
	for _, c := range assignedChores {
		assignedMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.getChoreMessageUrl(c))
	}
	if len(assignedChores) > 0 {
		embed := discordgo.MessageEmbed{
			Title:       "Your assigned chores",
			Description: assignedMd,
			Color:       ui.colors.OrangeColor,
		}
		embeds = append(embeds, &embed)
	}

	// get acked chores for the user
	ackedChores, err := ui.storage.GetAckedChoresForUser(userId)
	if err != nil {
		ui.logger.Error("failed to get acked chores", "error", err, "user_id", userId)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	ackedMd := ""
	for _, c := range ackedChores {
		ackedMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.getChoreMessageUrl(c))
	}
	if len(ackedChores) > 0 {
		embed := discordgo.MessageEmbed{
			Title:       "Your acknowledged chores",
			Description: ackedMd,
			Color:       ui.colors.GreenColor,
		}
		embeds = append(embeds, &embed)
	}

	if len(embeds) == 0 {
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title:       "No chores found!",
			Description: "You have no chores assigned or acknowledged.",
			Color:       ui.colors.GreenColor,
		})
	}

	r := &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Here are your chores:",
			Embeds:  embeds,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	}
	ui.discord.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) helpedChore(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to log work for chore."
	choreId, err := getChoreIdFromButton(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	userId := i.Interaction.Member.User.ID
	_, err = ui.storage.GetWorkLogForChoreAndUser(choreId, userId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			chore, err := ui.storage.GetChore(choreId)
			if err != nil {
				ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
				s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
				return
			}
			wl := storage.WorkLog{
				ChoreId:      chore.ID,
				UserId:       userId,
				TimeSpentMin: chore.EstimatedTimeMin,
				SelfReported: true,
			}
			_, err = ui.storage.SaveWorkLog(wl)
			if err != nil {
				ui.logger.Error("failed to save work log", "error", err, "chore_id", choreId, "user_id", userId)
				s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
				return
			}
			r := simpleContainerizedInteractionResponse(fmt.Sprintf("Logged work for chore `id: %d`.", choreId), &ui.colors.GreenColor)
			s.InteractionRespond(i.Interaction, r)
			err = ui.updateChoreMessage(chore)
			if err != nil {
				ui.logger.Error("failed to update chore message", "error", err, "chore_id", chore.ID)
				s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
				return
			}
			return
		} else {
			ui.logger.Error("failed to get work log", "error", err, "chore_id", choreId, "user_id", userId)
			s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
			return
		}
	}
	r := simpleContainerizedInteractionResponse(fmt.Sprintf("You already have work logged for chore `id: %d`.", choreId), &ui.colors.RedColor)
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) doneChore(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to complete chore."
	choreId, err := getChoreIdFromButton(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	if chore.Completed != nil {
		r := simpleContainerizedInteractionResponse(fmt.Sprintf("Chore `id: %d` has already been completed.", choreId), &ui.colors.RedColor)
		s.InteractionRespond(i.Interaction, r)
		return
	}
	if chore.Cancelled != nil {
		r := simpleContainerizedInteractionResponse(fmt.Sprintf("Chore `id: %d` has been cancelled.", choreId), &ui.colors.RedColor)
		s.InteractionRespond(i.Interaction, r)
		return
	}
	chore.Complete()
	_, err = ui.storage.SaveChore(chore)
	if err != nil {
		ui.logger.Error("failed to save chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` has been completed.", choreId), &ui.colors.GreenColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)

	ass, err := ui.storage.GetChoreAssignments(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore assignments", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	for _, a := range ass {
		if a.Refused != nil || a.Acked != nil {
			continue
		}
		if a.Timeouted == nil {
			a.Timeout()
		}
		_, err := ui.storage.SaveChoreAssignment(a)
		if err != nil {
			ui.logger.Error("failed to save chore assignment", "error", err, "chore_id", choreId)
			s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
			return
		}
	}

	for _, a := range ass {
		if a.Acked == nil {
			continue
		}
		wl := storage.WorkLog{
			ChoreId:      chore.ID,
			UserId:       a.UserId,
			TimeSpentMin: chore.EstimatedTimeMin,
		}
		_, err := ui.storage.SaveWorkLog(wl)
		if err != nil {
			ui.logger.Error("failed to save work log", "error", err, "chore_id", choreId, "user_id", a.UserId)
		}
		err = ui.sendDM(a.UserId, &discordgo.MessageSend{
			Content: fmt.Sprintf("Chore `id: %d` has been completed %s. Thank you for your work!\nYou spent `%d` minutes on this chore (which was the estimate of the chore creator).", choreId, ui.getChoreMessageUrl(chore), wl.TimeSpentMin),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.SuccessButton,
							Label:    "Change Time Spent",
							CustomID: reportTimeSpentClick + fmt.Sprint(chore.ID),
						},
					},
				},
			},
		})
		if err != nil {
			ui.logger.Error("failed to send DM", "error", err, "user_id", a.UserId)
			s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
			return
		}
	}

	err = ui.sendDM(chore.CreatorId, &discordgo.MessageSend{
		Content: fmt.Sprintf("Chore `id: %d` has been completed %s.", choreId, ui.getChoreMessageUrl(chore)),
	})
	if err != nil {
		ui.logger.Error("failed to send DM", "error", err, "user_id", chore.CreatorId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	err = ui.updateChoreMessage(chore)
	if err != nil {
		ui.logger.Error("failed to update chore message", "error", err, "chore_id", chore.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
}
