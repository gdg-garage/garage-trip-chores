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

type SummaryRunner interface {
	RunOnce(ctx context.Context) error
}

type Ui struct {
	storage       *storage.Storage
	logger        *slog.Logger
	chores        *chores.ChoresLogic
	discord       *discordgo.Session
	conf          Config
	colors        Colors
	broadcaster   EventBroadcaster
	summaryRunner SummaryRunner
}

func (ui *Ui) SetSummaryRunner(runner SummaryRunner) {
	ui.summaryRunner = runner
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

func getInteractionUserId(i *discordgo.InteractionCreate) string {
	if i == nil || i.Interaction == nil {
		return ""
	}
	if i.Interaction.User != nil {
		return i.Interaction.User.ID
	}
	if i.Interaction.Member != nil && i.Interaction.Member.User != nil {
		return i.Interaction.Member.User.ID
	}
	return ""
}

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
	ui.emitChoreEvent(eventType, chore, nil)
}

// EmitWorkLogEvent emits a chore event that also carries the work log entry
// which caused it (worklog_added / worklog_updated).
func (ui *Ui) EmitWorkLogEvent(eventType string, chore storage.Chore, wl storage.WorkLog) {
	ui.emitChoreEvent(eventType, chore, &wl)
}

func (ui *Ui) emitChoreEvent(eventType string, chore storage.Chore, wl *storage.WorkLog) {
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
		Type:    storageEventType,
		Chore:   &chore,
		WorkLog: wl,
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
			go func(creatorId, choreName string, choreId uint, msgUrl string) {
				_ = ui.SendDM(creatorId, &discordgo.MessageSend{
					Content: fmt.Sprintf("Your chore `%s` (id: `%d`) was scheduled and published in <#%s>.\n%s", choreName, choreId, ui.conf.DiscordChannelId, msgUrl),
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								&discordgo.Button{
									Style:    discordgo.SuccessButton,
									Label:    "Done!",
									CustomID: DoneButtonClick + fmt.Sprint(choreId),
								},
								&discordgo.Button{
									Style:    discordgo.DangerButton,
									Label:    "Cancel",
									CustomID: CancelButtonClick + fmt.Sprint(choreId),
								},
							},
						},
					},
				})
			}(c.CreatorId, c.Name, c.ID, messageUrl)
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
	userId := getInteractionUserId(i)
	ui.logger.Info("scheduleChore button clicked", "button_id", buttonId, "chore_id", choreId, "user_id", userId)

	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		if respErr := s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText)); respErr != nil {
			ui.logger.Error("failed to respond to schedule interaction", "error", respErr, "chore_id", choreId)
		}
		return
	}

	// Defer message update immediately to prevent Discord interaction 3-second timeout (code 10062)
	isDeferred := false
	if respErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); respErr != nil {
		ui.logger.Warn("failed to defer schedule chore interaction, falling back to direct respond", "error", respErr, "chore_id", choreId)
	} else {
		isDeferred = true
	}

	sendResponse := func(r *discordgo.InteractionResponse) {
		if isDeferred {
			var edit discordgo.WebhookEdit
			if r.Data != nil {
				if r.Data.Content != "" {
					edit.Content = &r.Data.Content
				}
				if len(r.Data.Components) > 0 {
					edit.Components = &r.Data.Components
				}
				if len(r.Data.Embeds) > 0 {
					edit.Embeds = &r.Data.Embeds
				}
			}
			if _, respErr := s.InteractionResponseEdit(i.Interaction, &edit); respErr != nil {
				ui.logger.Error("failed to edit schedule chore interaction response", "error", respErr, "chore_id", choreId)
			}
		} else {
			r.Type = discordgo.InteractionResponseUpdateMessage
			if respErr := s.InteractionRespond(i.Interaction, r); respErr != nil {
				ui.logger.Error("failed to respond to schedule chore interaction", "error", respErr, "chore_id", choreId)
			}
		}
	}

	c, err := ui.storage.GetChore(choreId)
	if err != nil {
		ui.logger.Error("failed to get chore", "error", err, "chore_id", choreId)
		sendResponse(ui.errorInteractionResponse(failedText))
		return
	}

	if c.MessageId != "" {
		ui.logger.Warn("chore already scheduled and published", "chore_id", choreId, "message_id", c.MessageId)
		r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` is already scheduled and published.", choreId), &ui.colors.OrangeColor)
		sendResponse(r)
		return
	}

	if c.DelayMin > 0 {
		publishAt := time.Now().Add(time.Duration(c.DelayMin) * time.Minute)
		_, err := ui.storage.CreateDelayedTask(c.ID, publishAt, c.DelayMin)
		if err != nil {
			ui.logger.Error("failed to create delayed task", "error", err, "chore_id", choreId)
			sendResponse(ui.errorInteractionResponse(failedText))
			return
		}

		ui.logger.Info("Chore scheduled with delay", "chore_id", choreId, "delay_min", c.DelayMin, "publish_at", publishAt)
		r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` was scheduled with a delay and will be sent in %d minutes (at %s).", choreId, c.DelayMin, publishAt.Format("15:04")), &ui.colors.GreenColor)
		r.Data.Components = append(r.Data.Components, discordgo.Container{
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.DangerButton,
							Label:    "Cancel",
							CustomID: CancelButtonClick + fmt.Sprint(choreId),
						},
					},
				},
			},
		})
		sendResponse(r)
		return
	}

	c, _, err = ui.PublishChore(c)
	if err != nil {
		ui.logger.Error("failed to publish chore", "error", err, "chore_id", choreId)
		sendResponse(ui.errorInteractionResponse(failedText))
		return
	}

	ui.logger.Info("Chore successfully published and scheduled", "chore_id", choreId, "message_id", c.MessageId)
	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` was scheduled and published.", choreId), &ui.colors.GreenColor)
	r.Data.Components = append(r.Data.Components, discordgo.Container{
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
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
			},
		},
	})

	sendResponse(r)
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
	_ = ui.storage.CancelDelayedTaskByChoreId(choreId)
	_ = ui.UpdateChoreMessage(chore)
	ui.EmitChoreEvent("chore_cancelled", chore)
	return chore, nil
}

func (ui *Ui) cancelChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to remove chore."
	choreId, err := getChoreIdFromCustomID(buttonId)
	userId := getInteractionUserId(i)
	ui.logger.Info("cancelChore button clicked", "button_id", buttonId, "chore_id", choreId, "user_id", userId)

	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	isDeferred := false
	if respErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	}); respErr != nil {
		ui.logger.Warn("failed to defer cancel chore interaction, falling back to direct respond", "error", respErr, "chore_id", choreId)
	} else {
		isDeferred = true
	}

	sendResp := func(r *discordgo.InteractionResponse) {
		if isDeferred {
			var edit discordgo.WebhookEdit
			if r.Data != nil && len(r.Data.Components) > 0 {
				edit.Components = &r.Data.Components
			}
			if _, respErr := s.InteractionResponseEdit(i.Interaction, &edit); respErr != nil {
				ui.logger.Error("failed to edit cancel chore interaction response", "error", respErr, "chore_id", choreId)
			}
		} else {
			r.Type = discordgo.InteractionResponseUpdateMessage
			if respErr := s.InteractionRespond(i.Interaction, r); respErr != nil {
				ui.logger.Error("failed to respond to cancel chore interaction", "error", respErr, "chore_id", choreId)
			}
		}
	}

	_, err = ui.CancelChore(choreId)
	if err != nil {
		ui.logger.Error("failed to cancel chore", "error", err, "chore_id", choreId)
		sendResp(ui.errorInteractionResponse(err.Error()))
		return
	}

	ui.logger.Info("Chore cancelled successfully", "chore_id", choreId, "user_id", userId)
	sendResp(simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` has been removed.", choreId), &ui.colors.RedColor))
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
	userId := getInteractionUserId(i)
	ui.logger.Info("rejectChore button clicked", "button_id", buttonId, "chore_id", choreId, "user_id", userId)

	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", buttonId)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	isDeferred := false
	if respErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
		},
	}); respErr != nil {
		ui.logger.Warn("failed to defer reject chore interaction, falling back to direct respond", "error", respErr, "chore_id", choreId)
	} else {
		isDeferred = true
	}

	sendResp := func(content string, isError bool) {
		var resp *discordgo.InteractionResponse
		if isError {
			resp = ui.errorInteractionResponse(content)
		} else {
			resp = simpleInteractionResponse(content)
		}

		if isDeferred {
			var edit discordgo.WebhookEdit
			if resp.Data != nil && len(resp.Data.Components) > 0 {
				edit.Components = &resp.Data.Components
			}
			if _, respErr := s.InteractionResponseEdit(i.Interaction, &edit); respErr != nil {
				ui.logger.Error("failed to edit reject chore interaction response", "error", respErr, "chore_id", choreId)
			}
		} else {
			if respErr := s.InteractionRespond(i.Interaction, resp); respErr != nil {
				ui.logger.Error("failed to respond to reject chore interaction", "error", respErr, "chore_id", choreId)
			}
		}
	}

	_, err = ui.RejectChore(choreId, userId)
	if err != nil {
		ui.logger.Error("failed to reject chore", "error", err, "chore_id", choreId, "user_id", userId)
		sendResp(err.Error(), true)
		return
	}

	ui.logger.Info("Chore rejected successfully", "chore_id", choreId, "user_id", userId)
	sendResp(fmt.Sprintf("Chore `%d` rejected\n\n*... Dissapointing*", choreId), false)
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
	userId := getInteractionUserId(i)
	ui.logger.Info("ackChore button clicked", "custom_id", customID, "chore_id", choreId, "user_id", userId)

	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", customID)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	// Immediately defer ephemeral response to prevent Discord 3-second interaction timeout (code 10062)
	isDeferred := false
	if respErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
		},
	}); respErr != nil {
		ui.logger.Warn("failed to defer ack chore interaction, falling back to direct respond", "error", respErr, "chore_id", choreId)
	} else {
		isDeferred = true
	}

	sendResp := func(content string, isError bool) {
		var resp *discordgo.InteractionResponse
		if isError {
			resp = ui.errorInteractionResponse(content)
		} else {
			resp = simpleInteractionResponse(content)
		}

		if isDeferred {
			var edit discordgo.WebhookEdit
			if resp.Data != nil && len(resp.Data.Components) > 0 {
				edit.Components = &resp.Data.Components
			}
			if _, respErr := s.InteractionResponseEdit(i.Interaction, &edit); respErr != nil {
				ui.logger.Error("failed to edit ack chore interaction response", "error", respErr, "chore_id", choreId)
			}
		} else {
			if respErr := s.InteractionRespond(i.Interaction, resp); respErr != nil {
				ui.logger.Error("failed to respond to ack chore interaction", "error", respErr, "chore_id", choreId)
			}
		}
	}

	c, _, err := ui.AckChore(choreId, userId)
	if err != nil {
		ui.logger.Error("failed to ack chore", "error", err, "chore_id", choreId, "user_id", userId)
		sendResp(failedText, true)
		return
	}

	ui.logger.Info("Chore acknowledged successfully", "chore_id", choreId, "user_id", userId)
	sendResp(fmt.Sprintf("Chore `%s` (id: `%d`) acknowledged.", c.Name, c.ID), false)
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
	if chore.DelayMin > 0 {
		choreDesc += fmt.Sprintf("\n**Delay**: %d min", chore.DelayMin)
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
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		if opt != nil {
			optionMap[opt.Name] = opt
		}
	}

	userId := getInteractionUserId(i)
	name := ""
	if opt, ok := optionMap["name"]; ok && opt != nil {
		name = strings.TrimSpace(opt.StringValue())
	}
	if name == "" {
		ui.logger.Error("chore_create called with empty or missing name", "user_id", userId)
		_ = ui.discord.InteractionRespond(i.Interaction, ui.errorInteractionResponse("Chore name is required."))
		return
	}

	defaultDeadline := time.Now().Add(24 * time.Hour) // Default deadline is 24 hours from creation
	chore := storage.Chore{
		Name:                 name,
		NecessaryWorkers:     uint(1),
		EstimatedTimeMin:     uint(10),
		AssignmentTimeoutMin: uint(15),
		Deadline:             &defaultDeadline,
		CreatorId:            userId,     // Discord ID of the user who created the chore
		Created:              time.Now(), // Timestamp when the chore was created
	}

	for k, v := range optionMap {
		if v == nil {
			continue
		}
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
		case "delay":
			if v.IntValue() > 0 {
				chore.DelayMin = uint(v.IntValue())
			}
		}
	}

	ui.logger.Info("Creating chore preview from discord slash command",
		"name", chore.Name,
		"creator_id", chore.CreatorId,
		"delay_min", chore.DelayMin,
		"necessary_workers", chore.NecessaryWorkers,
		"estimated_time_min", chore.EstimatedTimeMin,
	)

	chore, err := ui.storage.SaveChore(chore)
	if err != nil {
		ui.logger.Error("failed to save chore in chore_create", "error", err, "name", chore.Name, "user_id", userId)
		if respErr := ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
				Components: []discordgo.MessageComponent{
					discordgo.TextDisplay{
						Content: "Failed to create chore",
					},
				},
			},
		}); respErr != nil {
			ui.logger.Error("failed to respond to chore_create with error", "error", respErr)
		}
		return
	}

	ui.logger.Info("Saved chore preview to storage", "chore_id", chore.ID, "name", chore.Name)

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

	seenSkills := make(map[string]bool)
	for _, r := range skills {
		if seenSkills[r] {
			continue
		}
		seenSkills[r] = true
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

	components := []discordgo.MessageComponent{
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
	}

	if len(capabilityOptions) > 0 {
		maxValues := len(capabilityOptions)
		if maxValues > 25 {
			maxValues = 25
		}
		components = append(components, discordgo.Container{
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
							MaxValues:   maxValues,
							Options:     capabilityOptions,
						},
					},
				},
			},
		})
	}

	scheduleLabel := "Schedule"
	if chore.DelayMin > 0 {
		scheduleLabel = fmt.Sprintf("Schedule (in %d min)", chore.DelayMin)
	}

	components = append(components, discordgo.Container{
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					&discordgo.Button{
						Style:    discordgo.SuccessButton,
						Label:    scheduleLabel,
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
	})

	err = ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:      discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
			Components: components,
		},
	})
	if err != nil {
		ui.logger.Error("failed to respond to chore_create interaction", "error", err, "chore_id", chore.ID, "user_id", userId)
	} else {
		ui.logger.Info("Responded to chore_create interaction with preview", "chore_id", chore.ID, "user_id", userId)
	}
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
		userId := getInteractionUserId(i)
		channelId := i.ChannelID

		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			cmdName := i.ApplicationCommandData().Name
			ui.logger.Info("Received application command", "command", cmdName, "channel_id", channelId, "user_id", userId)

			allowed := channelId == ui.conf.DiscordChannelId
			if !allowed && ui.conf.DiscordChannelId != "" {
				if ch, err := s.State.Channel(channelId); err == nil && ch != nil && ch.ParentID == ui.conf.DiscordChannelId {
					allowed = true
				} else if ch, err := s.Channel(channelId); err == nil && ch != nil && ch.ParentID == ui.conf.DiscordChannelId {
					allowed = true
				}
			}

			// chore_create and chore_summary interactions are ephemeral or safe from any channel
			if !allowed && cmdName != "chore_create" && cmdName != "chore_summary" {
				ui.logger.Warn("Command rejected due to channel restriction", "command", cmdName, "channel_id", channelId, "expected_channel_id", ui.conf.DiscordChannelId, "user_id", userId)
				if err := s.InteractionRespond(i.Interaction, simpleInteractionResponse("This command can only be used in <#"+ui.conf.DiscordChannelId+"> channel.")); err != nil {
					ui.logger.Error("failed to respond to wrong channel interaction", "error", err, "channel_id", channelId)
				}
				return
			}

			switch cmdName {
			case "chore_create":
				ui.choreCreate(i)
			case "chore_summary":
				ui.choreSummary(i)
			case "chores":
				ui.choresList(i)
			case "chores_open":
				ui.choresOpen(i)
			case "chores_completed":
				ui.choresCompleted(i)
			case "stats":
				ui.stats(i)
			default:
				ui.logger.Warn("Unknown application command", "command", cmdName)
			}

		case discordgo.InteractionMessageComponent:
			data := i.MessageComponentData()
			ui.logger.Info("Received message component interaction", "custom_id", data.CustomID, "component_type", data.ComponentType, "channel_id", channelId, "user_id", userId)

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
			case strings.HasPrefix(data.CustomID, SkillsSelectMenu):
				ui.handleSkillsSelect(data.CustomID, s, i)
			default:
				ui.logger.Warn("Unhandled message component interaction", "custom_id", data.CustomID)
			}

		case discordgo.InteractionModalSubmit:
			data := i.ModalSubmitData()
			ui.logger.Info("Received modal submit interaction", "custom_id", data.CustomID, "channel_id", channelId, "user_id", userId)

			switch {
			case strings.HasPrefix(data.CustomID, ReportTimeSpentModal):
				ui.reportTimeSpent(s, i)
			case strings.HasPrefix(data.CustomID, EditChoreModal):
				ui.editChore(s, i)
			default:
				ui.logger.Warn("Unhandled modal submit interaction", "custom_id", data.CustomID)
			}
		}
	})

	// 3. Set the intent to receive GuildMessages, Guilds, and DirectMessages.
	// This is necessary for the bot to function correctly, especially for commands and DM interactions.
	ui.discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds | discordgo.IntentsDirectMessages

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
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "delay",
					Description: "Delay in minutes before sending and scheduling the task. [0]",
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
		{
			Name:        "chore_summary",
			Description: "Triggers the LLM chore summary and posts it to the chore channel.",
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
				if event.Type == storage.TaskUpdated || event.Type == storage.TaskDone || event.Type == storage.TaskAssigned || event.Type == storage.TaskAcked || event.Type == storage.TaskRefused || event.Type == storage.TaskTimeout || event.Type == storage.TaskCancelled || event.Type == storage.WorklogUpdated || event.Type == storage.WorklogAdded {
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
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	userId := getInteractionUserId(i)

	value := ""
	if userId != "" {
		if wl, err := ui.storage.GetWorkLogForChoreAndUser(choreId, userId); err == nil {
			value = fmt.Sprint(wl.TimeSpentMin)
		} else if chore, err := ui.storage.GetChore(choreId); err == nil && chore.EstimatedTimeMin > 0 {
			value = fmt.Sprint(chore.EstimatedTimeMin)
		}
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
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
							Value:       value,
							Required:    true,
						},
					},
				},
			},
		},
	})
	if err != nil {
		ui.logger.Error("failed to send modal", "error", err, "chore_id", choreId)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
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
		wl.SelfReported = true
	}

	wl, err = ui.storage.SaveWorkLog(wl)
	if err != nil {
		return wl, fmt.Errorf("failed to save work log: %w", err)
	}

	go func() {
		if err := ui.UpdateChoreMessage(chore); err != nil {
			ui.logger.Error("failed to asynchronously update chore message after time report", "error", err, "chore_id", chore.ID)
		}
	}()
	ui.EmitWorkLogEvent("worklog_updated", chore, wl)
	return wl, nil
}

func (ui *Ui) reportTimeSpent(s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to report time spent."
	data := i.Interaction.ModalSubmitData()

	choreId, err := getChoreIdFromCustomID(data.CustomID)
	if err != nil {
		ui.logger.Error("failed to parse chore ID from modal", "error", err, "custom_id", data.CustomID)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	timeSpentStr := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
	timeSpent, err := strconv.Atoi(timeSpentStr)
	if err != nil {
		ui.logger.Error("failed to parse time spent", "error", err, "input", timeSpentStr)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	userId := getInteractionUserId(i)
	if userId == "" {
		ui.logger.Error("failed to identify user from modal submit interaction", "chore_id", choreId)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	_, err = ui.ReportTimeSpent(choreId, userId, uint(timeSpent))
	if err != nil {
		ui.logger.Error("failed to save work log", "error", err, "chore_id", choreId, "user_id", userId)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("Updated time spent on chore `id: %d` to `%d` min.", choreId, timeSpent), &ui.colors.GreenColor)
	if err := s.InteractionRespond(i.Interaction, r); err != nil {
		ui.logger.Error("failed to respond to time spent interaction", "error", err, "chore_id", choreId, "user_id", userId)
	}
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

func (ui *Ui) choreSummary(i *discordgo.InteractionCreate) {
	if err := ui.discord.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		ui.logger.Error("failed to defer choreSummary interaction", "error", err)
	}

	if ui.summaryRunner == nil {
		content := "LLM summarizer is not configured."
		if _, err := ui.discord.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content,
		}); err != nil {
			ui.logger.Error("failed to edit choreSummary interaction response", "error", err)
		}
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		if err := ui.summaryRunner.RunOnce(ctx); err != nil {
			ui.logger.Error("Manual chore summary failed", "error", err)
			content := fmt.Sprintf("Failed to generate summary: %v", err)
			if _, err := ui.discord.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
				Content: &content,
			}); err != nil {
				ui.logger.Error("failed to edit choreSummary interaction response", "error", err)
			}
			return
		}

		content := "Chores summary generated and posted successfully to the chores channel!"
		if _, err := ui.discord.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Content: &content,
		}); err != nil {
			ui.logger.Error("failed to edit choreSummary interaction response", "error", err)
		}
	}()
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
		if c.MessageId == "" {
			continue
		}
		openMd += fmt.Sprintf("* %s (id: `%d`) %s\n", c.Name, c.ID, ui.GetChoreMessageUrl(c))
	}
	if openMd == "" {
		openMd = "No open chores at the moment."
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
	userId := getInteractionUserId(i)
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

	ui.EmitWorkLogEvent("worklog_added", chore, wl)
	return wl, nil
}

func (ui *Ui) helpedChore(d string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	failedText := "Failed to log work for chore."
	choreId, err := getChoreIdFromCustomID(d)
	userId := getInteractionUserId(i)
	ui.logger.Info("helpedChore button clicked", "custom_id", d, "chore_id", choreId, "user_id", userId)

	if err != nil {
		ui.logger.Error("failed to parse chore ID from button", "error", err, "custom_id", d)
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	isDeferred := false
	if respErr := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral | discordgo.MessageFlagsIsComponentsV2,
		},
	}); respErr != nil {
		ui.logger.Warn("failed to defer helped chore interaction, falling back to direct respond", "error", respErr, "chore_id", choreId)
	} else {
		isDeferred = true
	}

	sendResp := func(r *discordgo.InteractionResponse) {
		if isDeferred {
			var edit discordgo.WebhookEdit
			if r.Data != nil && len(r.Data.Components) > 0 {
				edit.Components = &r.Data.Components
			}
			if _, respErr := s.InteractionResponseEdit(i.Interaction, &edit); respErr != nil {
				ui.logger.Error("failed to edit helped chore interaction response", "error", respErr, "chore_id", choreId)
			}
		} else {
			if respErr := s.InteractionRespond(i.Interaction, r); respErr != nil {
				ui.logger.Error("failed to respond to helped chore interaction", "error", respErr, "chore_id", choreId)
			}
		}
	}

	_, err = ui.HelpedChore(choreId, userId)
	if err != nil {
		ui.logger.Error("failed to log work for chore", "error", err, "chore_id", choreId, "user_id", userId)
		sendResp(simpleContainerizedInteractionResponse(fmt.Sprintf("You already have work logged for chore `id: %d`.", choreId), &ui.colors.RedColor))
		return
	}

	ui.logger.Info("Chore work logged successfully", "chore_id", choreId, "user_id", userId)
	sendResp(simpleContainerizedInteractionResponse(fmt.Sprintf("Logged work for chore `id: %d`.", choreId), &ui.colors.GreenColor))
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
		_ = s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	chore, err := ui.CompleteChore(choreId)
	if err != nil {
		if strings.Contains(err.Error(), "already been completed") {
			ui.logger.Info("chore already completed when done button clicked", "chore_id", choreId)
			r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` has already been completed.", choreId), &ui.colors.GreenColor)
			r.Type = discordgo.InteractionResponseUpdateMessage
			_ = s.InteractionRespond(i.Interaction, r)
			return
		}
		ui.logger.Error("failed to complete chore", "error", err, "chore_id", choreId)
		r := ui.errorInteractionResponse(err.Error())
		r.Type = discordgo.InteractionResponseUpdateMessage
		_ = s.InteractionRespond(i.Interaction, r)
		return
	}

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` `%s` has been completed.", choreId, chore.Name), &ui.colors.GreenColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	if err := s.InteractionRespond(i.Interaction, r); err != nil {
		ui.logger.Error("failed to respond to doneChore interaction", "error", err, "chore_id", choreId)
	}
}

func (ui *Ui) RunDelayedTaskScheduler(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			ui.logger.Debug("Delayed task scheduler stopped: context cancelled")
			return
		case <-ticker.C:
			ui.ProcessPendingDelayedTasks()
		}
	}
}

func (ui *Ui) ProcessPendingDelayedTasks() {
	tasks, err := ui.storage.GetPendingDelayedTasks(time.Now())
	if err != nil {
		ui.logger.Error("failed to get pending delayed tasks", "error", err)
		return
	}

	for _, t := range tasks {
		chore, err := ui.storage.GetChore(t.ChoreID)
		if err != nil {
			ui.logger.Error("failed to get chore for delayed task", "chore_id", t.ChoreID, "error", err)
			continue
		}

		if chore.Cancelled != nil || chore.Completed != nil {
			ui.logger.Info("Chore cancelled or completed before delayed execution, marking executed", "chore_id", chore.ID)
			_ = ui.storage.MarkDelayedTaskExecuted(t.ID)
			continue
		}

		if chore.MessageId != "" {
			ui.logger.Info("Chore already published, marking executed", "chore_id", chore.ID)
			_ = ui.storage.MarkDelayedTaskExecuted(t.ID)
			continue
		}

		_, _, err = ui.PublishChore(chore)
		if err != nil {
			ui.logger.Error("failed to publish delayed chore", "chore_id", chore.ID, "error", err)
			continue
		}

		err = ui.storage.MarkDelayedTaskExecuted(t.ID)
		if err != nil {
			ui.logger.Error("failed to mark delayed task executed", "task_id", t.ID, "error", err)
		}
	}
}

