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

type EventBroadcaster interface {
	BroadcastChoreEvent(eventType string, chore storage.Chore, assignments []storage.ChoreAssignment, worklogs []storage.WorkLog)
}

type Ui struct {
	storage     *storage.Storage
	logger      *slog.Logger
	chores      *chores.ChoresLogic
	discord     *discordgo.Session
	conf        Config
	colors      Colors
	broadcaster EventBroadcaster
}

const (
	ButtonClickSuffix    = "_button_click:"
	AckButtonClick       = "ack" + ButtonClickSuffix
	CancelButtonClick    = "cancel" + ButtonClickSuffix
	DeleteButtonClick    = "delete" + ButtonClickSuffix
	DoneButtonClick      = "done" + ButtonClickSuffix
	EditButtonClick      = "edit" + ButtonClickSuffix
	RejectButtonClick    = "reject" + ButtonClickSuffix
	ScheduleButtonClick  = "schedule" + ButtonClickSuffix
	HelpedButtonClick    = "helped" + ButtonClickSuffix
	ReportTimeSpentClick = "report_time_spent" + ButtonClickSuffix

	ModalSubmitSuffix    = "_modal_submit:"
	ReportTimeSpentModal = "report_time_spent" + ModalSubmitSuffix
	EditChoreModal       = "edit" + ModalSubmitSuffix

	SelectMenuSuffix = "_select_menu:"
	SkillsSelectMenu = "skills" + SelectMenuSuffix
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

func getChoreIdFromCustomID(customID string) (uint, error) {
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

func (ui *Ui) SendDM(discordId string, message *discordgo.MessageSend) error {
	if ui.discord == nil || discordId == "" {
		return nil
	}
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

func (ui *Ui) GetChoreMessageUrl(c storage.Chore) string {
	return fmt.Sprintf("https://discord.com/channels/%s/%s/%s", ui.storage.GetDiscordGuildId(), ui.conf.DiscordChannelId, c.MessageId)
}

func (ui *Ui) SetBroadcaster(b EventBroadcaster) {
	ui.broadcaster = b
}

func (ui *Ui) EmitChoreEvent(eventType string, chore storage.Chore) {
	var storageEventType storage.EventType
	switch eventType {
	case "chore_created":
		storageEventType = storage.TaskCreated
	case "chore_updated":
		storageEventType = storage.TaskUpdated
	case "chore_reassigned":
		storageEventType = storage.TaskAssigned
	case "chore_claimed":
		storageEventType = storage.TaskAcked
	case "chore_rejected":
		storageEventType = storage.TaskRefused
	case "chore_completed":
		storageEventType = storage.TaskDone
	case "chore_cancelled":
		storageEventType = storage.TaskCancelled
	default:
		storageEventType = storage.EventType(eventType)
	}

	ui.storage.Events.Publish(storage.Event{
		Type:  storageEventType,
		Chore: &chore,
	})

	if ui.broadcaster != nil {
		assignments, _ := ui.storage.GetChoreAssignments(chore.ID)
		worklogs, _ := ui.storage.GetWorkLogsForChore(chore.ID)
		ui.broadcaster.BroadcastChoreEvent(eventType, chore, assignments, worklogs)
	}
}

func (ui *Ui) PublishChore(c storage.Chore) (storage.Chore, []storage.ChoreAssignment, error) {
	var err error
	if c.ID == 0 {
		c, err = ui.storage.SaveChore(c)
		if err != nil {
			return c, nil, fmt.Errorf("failed to save chore: %w", err)
		}
	}

	users, err := ui.storage.GetPresentUsers()
	if err != nil {
		ui.logger.Error("Error getting present users", "error", err)
		return c, nil, fmt.Errorf("error getting present users: %w", err)
	}
	ass, err := ui.chores.AssignChoresToUsers(users, c)
	if err != nil {
		ui.logger.Error("Error assigning chores to users", "error", err)
		return c, nil, fmt.Errorf("error assigning chores to users: %w", err)
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

	if ui.discord != nil {
		m, err := ui.discord.ChannelMessageSendComplex(ui.conf.DiscordChannelId, &discordgo.MessageSend{
			Content: c.Name,
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.PrimaryButton,
							Label:    "Ack",
							CustomID: AckButtonClick + fmt.Sprint(c.ID),
						},
						&discordgo.Button{
							Style:    discordgo.SecondaryButton,
							Label:    "Reject",
							CustomID: RejectButtonClick + fmt.Sprint(c.ID),
						},
					},
				},
			},
			Embeds: embeds,
		})
		if err != nil {
			ui.logger.Error("failed to send public chore message", "error", err, "chore_id", c.ID)
			return c, ass, fmt.Errorf("failed to send public chore message: %w", err)
		}

		c.MessageId = m.ID
		c, err = ui.storage.SaveChore(c)
		if err != nil {
			ui.logger.Error("failed to save chore with message ID", "error", err, "chore_id", c.ID)
			return c, ass, fmt.Errorf("failed to save chore with message ID: %w", err)
		}
		ui.logger.Info("Chore scheduled and published", "chore_id", c.ID, "message_id", m.ID)

		if c.CreatorId != "" {
			messageUrl := ui.GetChoreMessageUrl(c)
			_ = ui.SendDM(c.CreatorId, &discordgo.MessageSend{
				Content: fmt.Sprintf("Your chore `%s` (id: `%d`) was scheduled and published in <#%s>.\n%s", c.Name, c.ID, ui.conf.DiscordChannelId, messageUrl),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							&discordgo.Button{
								Style:    discordgo.SuccessButton,
								Label:    "Done!",
								CustomID: DoneButtonClick + fmt.Sprint(c.ID),
							},
							&discordgo.Button{
								Style:    discordgo.DangerButton,
								Label:    "Cancel",
								CustomID: CancelButtonClick + fmt.Sprint(c.ID),
							},
						},
					},
				},
			})
		}
	} else {
		c, err = ui.storage.SaveChore(c)
		if err != nil {
			return c, ass, err
		}
	}

	ui.EmitChoreEvent("chore_created", c)
	return c, ass, nil
}

func (ui *Ui) scheduleChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to schedule chore."
	choreId, err := getChoreIdFromCustomID(buttonId)
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

	c, _, err = ui.PublishChore(c)
	if err != nil {
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
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

func (ui *Ui) editChoreModal(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to edit chore."
	choreId, err := getChoreIdFromCustomID(buttonId)
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

	deadlineMin := 0
	if chore.Deadline != nil {
		deadlineMin = int(time.Until(*chore.Deadline).Minutes())
	}

	// open edit modal
	err = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: EditChoreModal + fmt.Sprint(choreId),
			Title:    fmt.Sprintf("Edit chore %d", choreId),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID:  "name",
							Label:     "Name",
							Style:     discordgo.TextInputShort,
							MinLength: 1,
							MaxLength: 100,
							Value:     chore.Name,
							Required:  true,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID:  "necessary_workers",
							Label:     "Necessary Workers",
							Style:     discordgo.TextInputShort,
							MinLength: 1,
							MaxLength: 10,
							Value:     fmt.Sprintf("%d", chore.NecessaryWorkers),
							Required:  true,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID:  "estimated_time_min",
							Label:     "Estimated Time (min)",
							Style:     discordgo.TextInputShort,
							MinLength: 1,
							MaxLength: 10,
							Value:     fmt.Sprintf("%d", chore.EstimatedTimeMin),
							Required:  true,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID:  "assignment_timeout_min",
							Label:     "Assignment Timeout (min)",
							Style:     discordgo.TextInputShort,
							MinLength: 1,
							MaxLength: 10,
							Value:     fmt.Sprintf("%d", chore.AssignmentTimeoutMin),
							Required:  true,
						},
					},
				},
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.TextInput{
							CustomID:  "deadline",
							Label:     "Deadline (min)",
							Style:     discordgo.TextInputShort,
							MinLength: 1,
							MaxLength: 10,
							Value:     fmt.Sprintf("%d", deadlineMin),
							Required:  true,
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

func (ui *Ui) CancelChore(choreId uint) (storage.Chore, error) {
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		return chore, fmt.Errorf("failed to get chore: %w", err)
	}
	if chore.Cancelled != nil {
		return chore, fmt.Errorf("chore has already been cancelled")
	}
	if chore.Completed != nil {
		return chore, fmt.Errorf("chore has been completed and cannot be cancelled")
	}

	t := time.Now()
	chore.Cancelled = &t
	chore, err = ui.storage.SaveChore(chore)
	if err != nil {
		return chore, fmt.Errorf("failed to save chore: %w", err)
	}

	_ = ui.storage.RemoveStorageAssignments(choreId)
	_ = ui.UpdateChoreMessage(chore)
	ui.EmitChoreEvent("chore_cancelled", chore)
	return chore, nil
}

func (ui *Ui) cancelChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to remove chore."
	choreId, err := getChoreIdFromCustomID(buttonId)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	_, err = ui.CancelChore(choreId)
	if err != nil {
		ui.logger.Error("failed to cancel chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(err.Error()))
		return
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` has been removed.", choreId), &ui.colors.RedColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) RejectChore(choreId uint, userId string) (storage.Chore, error) {
	c, err := ui.storage.GetChore(choreId)
	if err != nil {
		return c, fmt.Errorf("failed to get chore: %w", err)
	}

	ass, err := ui.storage.GetChoreAssignment(c.ID, userId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c, fmt.Errorf("chore cannot be rejected, you are not assigned to it")
		}
		return c, fmt.Errorf("failed to get chore assignment: %w", err)
	}

	ass.Refuse()
	_, err = ui.storage.SaveChoreAssignment(ass)
	if err != nil {
		return c, fmt.Errorf("failed to save chore assignment: %w", err)
	}

	users, err := ui.storage.GetPresentUsers()
	if err == nil {
		_, _ = ui.chores.AssignChoresToUsers(users, c)
	}

	_ = ui.UpdateChoreMessage(c)
	ui.EmitChoreEvent("chore_rejected", c)
	return c, nil
}

func (ui *Ui) rejectChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to reject chore."
	choreId, err := getChoreIdFromCustomID(buttonId)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	_, err = ui.RejectChore(choreId, i.Member.User.ID)
	if err != nil {
		ui.logger.Error("failed to reject chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(err.Error()))
		return
	}

	s.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("Chore `%d` rejected\n\n*... Dissapointing*", choreId)))
}

func (ui *Ui) AckChore(choreId uint, userId string) (storage.Chore, storage.ChoreAssignment, error) {
	var ass storage.ChoreAssignment
	c, err := ui.storage.GetChore(choreId)
	if err != nil {
		return c, ass, fmt.Errorf("failed to get chore: %w", err)
	}

	ass, err = ui.storage.GetChoreAssignment(choreId, userId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ass, err = ui.storage.AssignChore(c, userId)
			if err != nil {
				return c, ass, fmt.Errorf("failed to assign chore: %w", err)
			}
			ass.Volunteered = true
			ass, err = ui.storage.SaveChoreAssignment(ass)
			if err != nil {
				return c, ass, fmt.Errorf("failed to save chore assignment: %w", err)
			}
		} else {
			return c, ass, fmt.Errorf("failed to get chore assignment: %w", err)
		}
	}

	ass.Ack()
	ass, err = ui.storage.SaveChoreAssignment(ass)
	if err != nil {
		return c, ass, fmt.Errorf("failed to save chore assignment: %w", err)
	}

	if ui.discord != nil && userId != "" {
		_ = ui.SendDM(userId, &discordgo.MessageSend{
			Content: fmt.Sprintf("Your acknowledged chore `id: %d` `%s` %s.", c.ID, c.Name, ui.GetChoreMessageUrl(c)),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.SuccessButton,
							Label:    "Done!",
							CustomID: DoneButtonClick + fmt.Sprint(c.ID),
						},
					},
				},
			},
		})
	}

	_ = ui.UpdateChoreMessage(c)
	ui.EmitChoreEvent("chore_claimed", c)
	return c, ass, nil
}

func (ui *Ui) ackChore(customID string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to acknowledge chore."
	choreId, err := getChoreIdFromCustomID(customID)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", customID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	c, _, err := ui.AckChore(choreId, i.Member.User.ID)
	if err != nil {
		ui.logger.Error("failed to ack chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	s.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("Chore `%s` (id: `%d`) acknowledged.", c.Name, c.ID)))
}

func (ui *Ui) UpdateChoreMessage(chore storage.Chore) error {
	if ui.discord == nil {
		return nil
	}
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
						CustomID: AckButtonClick + fmt.Sprint(chore.ID),
					},
					&discordgo.Button{
						Style:    discordgo.SecondaryButton,
						Label:    "Reject",
						CustomID: RejectButtonClick + fmt.Sprint(chore.ID),
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
						CustomID: HelpedButtonClick + fmt.Sprint(chore.ID),
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
	necessaryCapabilities := strings.Join(chore.GetCapabilities(), ", ")
	choreDesc := fmt.Sprintf("### Name: `%s`\n"+
		"**Creator**: <@%s>\n"+
		"**ID**: `%d`\n"+
		"**Estimated Time (min)**: `%d`\n"+
		"**Necessary Workers**: `%d`\n"+
		"**Assignment Timeout (min)**: `%d`",
		name, chore.CreatorId, chore.ID, chore.EstimatedTimeMin, chore.NecessaryWorkers,
		chore.AssignmentTimeoutMin)

	if necessaryCapabilities != "" {
		choreDesc += fmt.Sprintf("\n**Necessary Capabilities**: `%s`", necessaryCapabilities)
	}
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

	minCapabilities := 0

	ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			// Content: "Please check the chore",
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2, // Makes the message visible only to the user who invoked the command.

			Components: []discordgo.MessageComponent{

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

				discordgo.Container{
					Components: []discordgo.MessageComponent{
						&discordgo.TextDisplay{
							Content: "Skills required for this chore:",
						},

						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{

								&discordgo.SelectMenu{
									CustomID:    SkillsSelectMenu + fmt.Sprint(chore.ID),
									Placeholder: "Required skills for this chore",
									MinValues:   &minCapabilities,
									MaxValues:   len(capabilityOptions), // Allow selecting all
									Options:     capabilityOptions,
								},
							},
						},
					},
				},

				discordgo.Container{
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								&discordgo.Button{
									Style:    discordgo.SuccessButton,
									Label:    "Schedule",
									CustomID: ScheduleButtonClick + fmt.Sprint(chore.ID),
								},
								&discordgo.Button{
									Style:    discordgo.SecondaryButton,
									Label:    "Edit",
									CustomID: EditButtonClick + fmt.Sprint(chore.ID),
								},
								&discordgo.Button{
									Style:    discordgo.DangerButton,
									Label:    "Delete",
									CustomID: DeleteButtonClick + fmt.Sprint(chore.ID),
								},
							},
						},
					},
				},
			},
		},
	})
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
	wg.Add(1)
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
			case strings.HasPrefix(data.CustomID, DeleteButtonClick) || strings.HasPrefix(data.CustomID, CancelButtonClick):
				ui.cancelChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, EditButtonClick):
				ui.editChoreModal(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, ScheduleButtonClick):
				ui.scheduleChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, RejectButtonClick):
				ui.rejectChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, AckButtonClick):
				ui.ackChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, DoneButtonClick):
				ui.doneChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, HelpedButtonClick):
				ui.helpedChore(data.CustomID, s, i)
			case strings.HasPrefix(data.CustomID, ReportTimeSpentClick):
				ui.reportTimeSpentButtonClick(data.CustomID, s, i)
			}
		}

		if i.Type == discordgo.InteractionModalSubmit {
			data := i.ModalSubmitData()
			switch {
			case strings.HasPrefix(data.CustomID, ReportTimeSpentModal):
				ui.reportTimeSpent(s, i)
			case strings.HasPrefix(data.CustomID, EditChoreModal):
				ui.editChore(s, i)
			}
		}

		if i.Type == discordgo.InteractionMessageComponent {
			data := i.MessageComponentData()
			switch {
			case strings.HasPrefix(data.CustomID, SkillsSelectMenu):
				ui.handleSkillsSelect(data.CustomID, s, i)
			}
		}

	})

	// 3. Set the intent to receive GuildMessages and Guilds.
	// This is necessary for the bot to function correctly, especially for commands.
	ui.discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds

	// 4. Open the WebSocket connection to Discord.

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

	// 6.a Start Event Listener loop
	go func() {
		sub := ui.storage.Events.Subscribe()
		defer ui.storage.Events.Unsubscribe(sub)
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-sub:
				if event.Type == storage.TaskUpdated || event.Type == storage.TaskDone || event.Type == storage.TaskAssigned || event.Type == storage.TaskAcked || event.Type == storage.TaskRefused || event.Type == storage.TaskTimeout || event.Type == storage.TaskCancelled {
					var choreToUpdate *storage.Chore
					if event.Chore != nil {
						choreToUpdate = event.Chore
					} else if event.Assignment != nil {
						chore, err := ui.storage.GetChore(event.Assignment.ChoreId)
						if err == nil {
							choreToUpdate = &chore
						}
					}
					
					if choreToUpdate != nil && choreToUpdate.MessageId != "" {
						err := ui.UpdateChoreMessage(*choreToUpdate)
						if err != nil {
							ui.logger.Error("failed to remotely update chore message from bus event", "error", err, "chore_id", choreToUpdate.ID)
						}
					}
				}
			}
		}
	}()

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

func (ui *Ui) handleSkillsSelect(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to update skills."

	choreId, err := getChoreIdFromCustomID(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from select menu", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	selectedSkills := i.MessageComponentData().Values
	chore.SetCapabilities(selectedSkills)

	chore, err = ui.storage.SaveChore(chore)
	if err != nil {
		ui.logger.Error("failed to save chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	_ = ui.UpdateChoreMessage(chore)
	ui.EmitChoreEvent("chore_updated", chore)

	r := simpleContainerizedInteractionResponse("Successfully updated skills for the chore.", &ui.colors.GreenColor)
	skillsMd := "### Skills\n"
	if len(selectedSkills) == 0 {
		skillsMd += "* No skills required\n"
	}
	for _, skill := range selectedSkills {
		skillsMd += fmt.Sprintf("* %s\n", skill)
	}
	container := &discordgo.Container{
		AccentColor: &ui.colors.GreenColor,
		Components: []discordgo.MessageComponent{
			&discordgo.TextDisplay{
				Content: skillsMd,
			},
		},
	}
	r.Data.Components = append(r.Data.Components, container)
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) EditChoreDetails(choreId uint, name string, necessaryWorkers, estimatedTimeMin, assignmentTimeoutMin uint, deadline *time.Time, capabilities []string) (storage.Chore, error) {
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		return chore, fmt.Errorf("failed to get chore: %w", err)
	}

	if name != "" {
		chore.Name = name
	}
	if necessaryWorkers > 0 {
		chore.NecessaryWorkers = necessaryWorkers
	}
	if estimatedTimeMin > 0 {
		chore.EstimatedTimeMin = estimatedTimeMin
	}
	chore.AssignmentTimeoutMin = assignmentTimeoutMin
	if deadline != nil {
		chore.Deadline = deadline
	}
	if capabilities != nil {
		chore.SetCapabilities(capabilities)
	}

	chore, err = ui.storage.SaveChore(chore)
	if err != nil {
		return chore, fmt.Errorf("failed to update chore: %w", err)
	}

	_ = ui.UpdateChoreMessage(chore)
	ui.EmitChoreEvent("chore_updated", chore)
	return chore, nil
}

func (ui *Ui) editChore(s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to edit chore."
	data := i.Interaction.ModalSubmitData()

	choreId, err := getChoreIdFromCustomID(data.CustomID)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from modal", "error", err, "custom_id", data.CustomID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	updatedName := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	updatedNecessaryWorkers, err := strconv.Atoi(data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value)
	if err != nil {
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	updatedEstimatedTimeMin, err := strconv.Atoi(data.Components[2].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value)
	if err != nil {
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	updatedAssignmentTimeoutMin, err := strconv.Atoi(data.Components[3].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value)
	if err != nil {
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	updatedDeadlineMin, err := strconv.Atoi(data.Components[4].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value)
	if err != nil {
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	deadline := time.Now().Add(time.Duration(updatedDeadlineMin) * time.Minute)
	chore, err := ui.EditChoreDetails(choreId, updatedName, uint(updatedNecessaryWorkers), uint(updatedEstimatedTimeMin), uint(updatedAssignmentTimeoutMin), &deadline, nil)
	if err != nil {
		ui.logger.Error("failed to update chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	successText := fmt.Sprintf("Successfully updated chore `id: %d`.", choreId)
	r := simpleContainerizedInteractionResponse(successText, &ui.colors.GreenColor)
	choreMd := ui.generateChoreMd(chore)
	container := &discordgo.Container{
		AccentColor: &ui.colors.GreenColor,
		Components: []discordgo.MessageComponent{
			&discordgo.TextDisplay{
				Content: choreMd,
			},
		},
	}
	r.Data.Components = append(r.Data.Components, container)
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) reportTimeSpentButtonClick(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to report time spent."

	choreId, err := getChoreIdFromCustomID(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	err = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: ReportTimeSpentModal + fmt.Sprint(choreId),
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

func (ui *Ui) ReportTimeSpent(choreId uint, userId string, timeSpentMin uint) (storage.WorkLog, error) {
	var wl storage.WorkLog
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		return wl, fmt.Errorf("failed to get chore: %w", err)
	}

	wl, err = ui.storage.GetWorkLogForChoreAndUser(choreId, userId)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			wl = storage.WorkLog{
				ChoreId:      chore.ID,
				UserId:       userId,
				TimeSpentMin: timeSpentMin,
				SelfReported: true,
			}
		} else {
			return wl, fmt.Errorf("failed to get work log: %w", err)
		}
	} else {
		wl.TimeSpentMin = timeSpentMin
	}

	wl, err = ui.storage.SaveWorkLog(wl)
	if err != nil {
		return wl, fmt.Errorf("failed to save work log: %w", err)
	}

	_ = ui.UpdateChoreMessage(chore)
	ui.EmitChoreEvent("worklog_updated", chore)
	return wl, nil
}

func (ui *Ui) reportTimeSpent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to report time spent."
	data := i.Interaction.ModalSubmitData()

	choreId, err := getChoreIdFromCustomID(data.CustomID)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from modal", "error", err, "custom_id", data.CustomID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	timeSpentStr := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	timeSpent, err := strconv.Atoi(timeSpentStr)
	if err != nil {
		ui.logger.Error("failed to parse time spent", "error", err, "input", timeSpentStr)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	userId := i.Interaction.User.ID
	_, err = ui.ReportTimeSpent(choreId, userId, uint(timeSpent))
	if err != nil {
		ui.logger.Error("failed to save work log", "error", err, "chore_id", choreId, "user_id", userId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("Updated time spent on chore `id: %d` to `%d` min.", choreId, timeSpent), &ui.colors.GreenColor)
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) stats(i *discordgo.InteractionCreate) {
	failedText := "Failed to get stats."
	embeds := []*discordgo.MessageEmbed{}

	usersStats, err := ui.storage.GetAggregatedStats()
	if err != nil {
		ui.logger.Error("failed to get aggregated stats", "error", err)
		ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	// Convert map to slice for sorting
	type kv struct {
		Key   string
		Value storage.AggregatedUserStats
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
		statsMd += fmt.Sprintf("```%0.f\t%0.f\t%0.f\t%0.f\t%0.f\t%0.f\t%d\t%.6f```\n",
			c.WorkedCount, c.WorkedMin, c.AssignedCount, c.AssignedMin, c.TotalCount, c.TotalMin, c.PresentTicks, c.NormalizedTotal)
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
		completedMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.GetChoreMessageUrl(c))
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
		openMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.GetChoreMessageUrl(c))
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
		assignedMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.GetChoreMessageUrl(c))
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
		ackedMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.GetChoreMessageUrl(c))
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

func (ui *Ui) HelpedChore(choreId uint, userId string) (storage.WorkLog, error) {
	var wl storage.WorkLog
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		return wl, fmt.Errorf("failed to get chore: %w", err)
	}

	existingWl, err := ui.storage.GetWorkLogForChoreAndUser(choreId, userId)
	if err == nil && existingWl.ID != 0 {
		return existingWl, fmt.Errorf("work already logged for this chore and user")
	}

	wl = storage.WorkLog{
		ChoreId:      chore.ID,
		UserId:       userId,
		TimeSpentMin: chore.EstimatedTimeMin,
		SelfReported: true,
	}
	wl, err = ui.storage.SaveWorkLog(wl)
	if err != nil {
		return wl, fmt.Errorf("failed to save work log: %w", err)
	}

	_ = ui.UpdateChoreMessage(chore)

	if ui.discord != nil && userId != "" {
		_ = ui.SendDM(userId, &discordgo.MessageSend{
			Content: fmt.Sprintf("Chore `id: %d` `%s` has been completed %s. Thank you for your work!\nYou spent `%d` minutes on this chore (which was the estimate of the chore creator).", choreId, chore.Name, ui.GetChoreMessageUrl(chore), wl.TimeSpentMin),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.SuccessButton,
							Label:    "Change Time Spent",
							CustomID: ReportTimeSpentClick + fmt.Sprint(chore.ID),
						},
					},
				},
			},
		})
	}

	ui.EmitChoreEvent("worklog_added", chore)
	return wl, nil
}

func (ui *Ui) helpedChore(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to log work for chore."
	choreId, err := getChoreIdFromCustomID(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	userId := i.Interaction.Member.User.ID
	_, err = ui.HelpedChore(choreId, userId)
	if err != nil {
		ui.logger.Error("failed to log work for chore", "error", err, "chore_id", choreId, "user_id", userId)
		s.InteractionRespond(i.Interaction, simpleContainerizedInteractionResponse(fmt.Sprintf("You already have work logged for chore `id: %d`.", choreId), &ui.colors.RedColor))
		return
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("Logged work for chore `id: %d`.", choreId), &ui.colors.GreenColor)
	s.InteractionRespond(i.Interaction, r)
}

func (ui *Ui) CompleteChore(choreId uint) (storage.Chore, error) {
	chore, err := ui.storage.GetChore(choreId)
	if err != nil {
		return chore, fmt.Errorf("failed to get chore: %w", err)
	}
	if chore.Completed != nil {
		return chore, fmt.Errorf("chore has already been completed")
	}
	if chore.Cancelled != nil {
		return chore, fmt.Errorf("chore has been cancelled")
	}

	chore.Complete()
	chore, err = ui.storage.SaveChore(chore)
	if err != nil {
		return chore, fmt.Errorf("failed to save chore: %w", err)
	}

	ass, err := ui.storage.GetChoreAssignments(choreId)
	if err == nil {
		for _, a := range ass {
			if a.Refused != nil || a.Acked != nil {
				continue
			}
			if a.Timeouted == nil {
				a.Timeout()
				_, _ = ui.storage.SaveChoreAssignment(a)
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
			_, _ = ui.storage.SaveWorkLog(wl)

			if ui.discord != nil && a.UserId != "" {
				_ = ui.SendDM(a.UserId, &discordgo.MessageSend{
					Content: fmt.Sprintf("Chore `id: %d` `%s` has been completed %s. Thank you for your work!\nYou spent `%d` minutes on this chore (which was the estimate of the chore creator).", choreId, chore.Name, ui.GetChoreMessageUrl(chore), wl.TimeSpentMin),
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								&discordgo.Button{
									Style:    discordgo.SuccessButton,
									Label:    "Change Time Spent",
									CustomID: ReportTimeSpentClick + fmt.Sprint(chore.ID),
								},
							},
						},
					},
				})
			}
		}
	}

	if ui.discord != nil && chore.CreatorId != "" {
		_ = ui.SendDM(chore.CreatorId, &discordgo.MessageSend{
			Content: fmt.Sprintf("Chore `id: %d`. `%s` has been completed %s.", choreId, chore.Name, ui.GetChoreMessageUrl(chore)),
		})
	}

	_ = ui.UpdateChoreMessage(chore)
	ui.EmitChoreEvent("chore_completed", chore)
	return chore, nil
}

func (ui *Ui) doneChore(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to complete chore."
	choreId, err := getChoreIdFromCustomID(d)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	chore, err := ui.CompleteChore(choreId)
	if err != nil {
		ui.logger.Error("failed to complete chore", "error", err, "chore_id", choreId)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(err.Error()))
		return
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` `%s` has been completed.", choreId, chore.Name), &ui.colors.GreenColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)
}
