package ui

import (
	"context"
	"fmt"
	"log"
	"log/slog"
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

	assignemntsMd := ""

	for _, a := range ass {
		assignemntsMd += fmt.Sprintf("<@%s> ", a.UserId)
	}

	choreMd := generateChoreMd(c)

	embeds := []*discordgo.MessageEmbed{}

	assignemntsEmbed := discordgo.MessageEmbed{
		Type:        discordgo.EmbedTypeRich,
		Description: choreMd,
	}
	embeds = append(embeds, &assignemntsEmbed)

	if len(ass) > 0 {
		assignemntsEmbed := discordgo.MessageEmbed{
			Type:        discordgo.EmbedTypeRich,
			Title:       "assignments",
			Description: assignemntsMd,
			Color:       ui.colors.OrangeColor,
		}
		embeds = append(embeds, &assignemntsEmbed)
	}

	// Send a public message to the channel announcing the scheduled chore
	m, err := s.ChannelMessageSendComplex(ui.conf.DiscordChannelId, &discordgo.MessageSend{
		// Flags:   discordgo.MessageFlagsIsComponentsV2,
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

			// TODO send in followup ephemeral message
			// discordgo.ActionsRow{
			// 	Components: []discordgo.MessageComponent{
			// 		&discordgo.Button{
			// 			Style:    discordgo.SuccessButton,
			// 			Label:    "Done!",
			// 			CustomID: "done_button_click",
			// 		},
			// 		&discordgo.Button{
			// 			Style:    discordgo.DangerButton,
			// 			Label:    "Cancel",
			// 			CustomID: "cancel_button_click",
			// 		},
			// 	},
			// },

			// discordgo.ActionsRow{
			// 	Components: []discordgo.MessageComponent{
			// 		&discordgo.SelectMenu{
			// 			CustomID:    "log_work_select_menu",
			// 			Placeholder: "Log time spent on this chore",
			// 			MaxValues:   1,
			// 			Options: []discordgo.SelectMenuOption{
			// 				{
			// 					Label:       "5 min",
			// 					Value:       "5",
			// 					Description: "Log 5 minutes of work.",
			// 					Default:     true, // Default option selected
			// 				},
			// 				{
			// 					Label:       "10 min",
			// 					Value:       "10",
			// 					Description: "Log 10 minutes of work.",
			// 				},
			// 				// TODO add more
			// 			},
			// 		},
			// 	},
			// },
		},
		Embeds: embeds,
	})
	if err != nil {
		ui.logger.Error("failed to send public chore message", "error", err, "chore_id", c.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	// TODO store message ID.
	ui.logger.Info("Chore scheduled and published", "chore_id", c.ID, "message_id", m.ID)

	r := simpleContainerizedInteractionResponse(fmt.Sprintf("This chore `id: %d` was scheduled and published.", choreId), &ui.colors.GreenColor)
	r.Type = discordgo.InteractionResponseUpdateMessage
	s.InteractionRespond(i.Interaction, r)
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
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}
	// TODO - probably send another ephermeral message with all buttons and fields
	s.InteractionRespond(i.Interaction, simpleInteractionResponse("Not implemented."))
}

func (ui *Ui) removeChore(buttonId string, s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	txt := "rejected!"
	_, err = ui.discord.ChannelMessageEditComplex(
		&discordgo.MessageEdit{
			Content: &txt,
			ID:      i.Message.ID,
			Channel: i.Message.ChannelID,
		},
	)
	if err != nil {
		ui.logger.Error("failed to edit chore message", "error", err, "chore_id", c.ID)
		s.InteractionRespond(i.Interaction, ui.errorInteractionResponse(failedText))
		return
	}

	s.InteractionRespond(i.Interaction, simpleInteractionResponse(fmt.Sprintf("Chore `%d` rejected\n\n*... Dissapointing*", c.ID)))
}

// TODO optionally add creator.
func generateChoreMd(chore storage.Chore) string {
	choreDesc := fmt.Sprintf("### Name: `%s`\n"+
		"**ID**: `%d`\n"+
		"**Estimated Time (min)**: `%d`\n"+
		"**Necessary Workers**: `%d`\n"+
		"**Assignment Timeout (min)**: `%d`",
		chore.Name, chore.ID, chore.EstimatedTimeMin, chore.NecessaryWorkers,
		chore.AssignmentTimeoutMin)

	if chore.Deadline != nil {
		choreDesc += fmt.Sprintf("\n**Deadline**: %s", chore.Deadline.Format(time.RFC3339))
	}

	// TODO creator

	return choreDesc
}

func (ui *Ui) choreCreate(i *discordgo.InteractionCreate) {
	// Respond to the slash command interaction.
	options := i.ApplicationCommandData().Options
	optionMap := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, opt := range options {
		optionMap[opt.Name] = opt
	}

	// TODO this is empty - it will  be filled out for the buttons
	// fmt.Println("target id", i.ApplicationCommandData().TargetID)

	// if i.Message != nil {
	// 	fmt.Println("message id", i.Message.ID)
	// } else {
	// 	fmt.Println("no message")
	// }

	chore := storage.Chore{
		Name:                 optionMap["name"].StringValue(),
		NecessaryWorkers:     uint(1),
		EstimatedTimeMin:     uint(10),
		AssignmentTimeoutMin: uint(10),
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

	choreDesc := generateChoreMd(chore)

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
							CustomID: "schedule_button_click:" + fmt.Sprint(chore.ID),
						},
						&discordgo.Button{
							Style:    discordgo.SecondaryButton,
							Label:    "Edit",
							CustomID: "edit_button_click:" + fmt.Sprint(chore.ID),
						},
						&discordgo.Button{
							Style:    discordgo.DangerButton,
							Label:    "Delete",
							CustomID: "delete_button_click:" + fmt.Sprint(chore.ID),
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
		if i.Interaction.ChannelID != ui.conf.DiscordChannelId {
			// If the interaction is not in a channel, we can't respond.
			s.InteractionRespond(i.Interaction, simpleInteractionResponse("This command can only be used in <#"+ui.conf.DiscordChannelId+"> channel."))
			return
		}

		if i.Interaction.Type == discordgo.InteractionApplicationCommand { // Ensure the interaction type is set correctly.
			// Check if the interaction is an ApplicationCommand (a slash command).
			switch i.ApplicationCommandData().Name {
			case "chore_create":
				ui.choreCreate(i) // Call the appropriate handler function.
			}
		}
		if i.Type == discordgo.InteractionMessageComponent {
			data := i.MessageComponentData()
			switch {

			case strings.HasPrefix(data.CustomID, "delete_button_click"):
				ui.removeChore(data.CustomID, s, i)

			case strings.HasPrefix(data.CustomID, "edit_button_click"):
				ui.editChore(data.CustomID, s, i)

			case strings.HasPrefix(data.CustomID, "schedule_button_click"):
				ui.scheduleChore(data.CustomID, s, i)

			case strings.HasPrefix(data.CustomID, "reject_button_click"):
				ui.rejectChore(data.CustomID, s, i)

				// case "hello_button_click":
				// 	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// 		Type: discordgo.InteractionResponseUpdateMessage,
				// 		Data: &discordgo.InteractionResponseData{
				// 			Content: "You clicked the button!",
				// 			Flags:   discordgo.MessageFlagsEphemeral,
				// 		},
				// 	})
				// 	// s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// 	// 	Type: discordgo.InteractionResponseChannelMessageWithSource,
				// 	// 	Data: &discordgo.InteractionResponseData{
				// 	// 		Content: "You clicked the button!",
				// 	// 		Flags:   discordgo.MessageFlagsEphemeral,
				// 	// 	},
				// 	// })
				// case "hello_select_menu":
				// 	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// 		Type: discordgo.InteractionResponseChannelMessageWithSource,
				// 		Data: &discordgo.InteractionResponseData{
				// 			Content: fmt.Sprintf("You selected: %s", data.Values),
				// 			Flags:   discordgo.MessageFlagsEphemeral,
				// 		},
				// 	})
				// case "nothing_button_click":
				// 	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// 		Type: discordgo.InteractionResponseDeferredMessageUpdate,
				// 	})
				// case "more_button_click":
				// 	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// 		Type: discordgo.InteractionResponseModal,
				// 		Data: &discordgo.InteractionResponseData{
				// 			CustomID: "more_modal",
				// 			Title:    "More Information",
				// 			Components: []discordgo.MessageComponent{
				// 				discordgo.ActionsRow{
				// 					Components: []discordgo.MessageComponent{
				// 						&discordgo.TextInput{
				// 							CustomID:    "more_text_input",
				// 							Label:       "Enter more information",
				// 							Style:       discordgo.TextInputShort,
				// 							Placeholder: "Type something here...",
				// 							Required:    true,
				// 							MinLength:   1,
				// 							MaxLength:   100,
				// 						},
				// 					},
				// 				},
				// 			},
				// 		},
				// 	})
			}
		}
		if i.Type == discordgo.InteractionModalSubmit {
			data := i.ModalSubmitData()
			if data.CustomID == "more_modal" {
				// Handle the modal submission.
				textInput := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput)
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("You entered: %s", textInput.Value),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
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

	// 5. Register the slash commands globally.
	// You can also register them for specific guilds for faster updates during development.
	fmt.Println("Registering commands...")

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
					Description: "The number of workers required to complete the chore.",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "estimated_time_min",
					Description: "The estimated time to complete the chore in minutes.",
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
					Description: "The deadline for the chore in minutes from now. If not set, the chore will not have a deadline.",
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
					Description: "The time in minutes after which the chore will be unassigned if not acked.",
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
	}

	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	for i, v := range commands {
		cmd, err := ui.discord.ApplicationCommandCreate(ui.discord.State.User.ID, ui.storage.GetDiscordGuidId(), v) // "" for global commands
		if err != nil {
			log.Fatalf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}
	fmt.Println("Commands registered successfully!")

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
		err := ui.discord.ApplicationCommandDelete(ui.discord.State.User.ID, ui.storage.GetDiscordGuidId(), v.ID) // "" for global commands
		if err != nil {
			ui.logger.Error("Cannot delete command", "name", v.Name, "error", err)
		}
	}
	ui.logger.Info("Commands unregistered.")
	return nil
}
