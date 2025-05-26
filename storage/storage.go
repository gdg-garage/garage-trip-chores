package storage

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	slogGorm "github.com/orandin/slog-gorm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Storage struct {
	db      *gorm.DB
	logger  *slog.Logger
	discord *discordgo.Session
	conf    Config
}

func dbConnect(conf Config, logger *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(conf.DbPath), &gorm.Config{
		Logger: slogGorm.New(slogGorm.WithHandler(logger.Handler())),
	})

	if err != nil {
		logger.Error("failed to connect the database", "path", conf.DbPath, "error", err)
		return nil, err
	}

	// Migrate the schema
	db.AutoMigrate(&Chore{}, &WorkLog{}, &ChoreAssignment{})
	return db, nil
}

func discordConnect(token string) (*discordgo.Session, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	err = dg.Open()
	if err != nil {
		return nil, err
	}

	return dg, nil
}

func New(conf Config, logger *slog.Logger) (*Storage, error) {
	db, err := dbConnect(conf, logger)
	if err != nil {
		return nil, err
	}

	dg, err := discordConnect(conf.DiscordToken)
	if err != nil {
		return nil, err
	}

	return &Storage{
		db:      db,
		logger:  logger,
		discord: dg,
		conf:    conf,
	}, nil
}

func choreCreate(st *Storage, s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	chore := Chore{
		Name:                 optionMap["name"].StringValue(),
		NecessaryWorkers:     uint(1),
		EstimatedTimeMin:     uint(10),
		AssignmentTimeoutMin: uint(10),
		CreatorId:            i.Member.User.ID,       // Discord ID of the user who created the chore
		CreatorHandle:        i.Member.User.Username, // Handle of the user who created the chore
		Created:              time.Now(),             // Timestamp when the chore was created
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

	// js, _ := json.Marshal(chore)

	// green := 0x00FF00  // Green color
	orange := 0xFFA500 // Orange color

	fields := []*discordgo.MessageEmbedField{
		// NecessaryCapabilities string // Comma separated list of capabilities

		{
			Name:  "Name",
			Value: chore.Name,
		},
		{
			Name:  "Estimated Time (min)",
			Value: fmt.Sprintf("%d", chore.EstimatedTimeMin),
		},
		{
			Name:  "Necessary Workers",
			Value: fmt.Sprintf("%d", chore.NecessaryWorkers),
		},
		{
			Name:  "Assignment Timeout (min)",
			Value: fmt.Sprintf("%d", chore.AssignmentTimeoutMin),
		},
		{
			Name:  "Creator",
			Value: fmt.Sprintf("%s (%s)", chore.CreatorHandle, chore.CreatorId),
		},
	}

	if chore.Deadline != nil {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "Deadline",
			Value: chore.Deadline.Format(time.RFC3339),
		})
	}

	capabilityOptions := []discordgo.SelectMenuOption{}

	capbilitiesMap := map[string]struct{}{}
	for _, cap := range chore.GetCapabilities() {
		capbilitiesMap[cap] = struct{}{}
	}

	skills, err := st.GetSkills()
	if err != nil {
		st.logger.Error("failed to get skills", "error", err)
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

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Please check the chore",
			Flags:   discordgo.MessageFlagsEphemeral, // Makes the message visible only to the user who invoked the command.
			Embeds: []*discordgo.MessageEmbed{
				// {
				// 	Title:       "Chore Created",
				// 	Description: string(js),
				// 	Color:       green,
				// },
				{
					Title:  "Chore Details",
					Color:  orange,
					Fields: fields,
				},
			},

			// Embeds: []*discordgo.MessageEmbed{
			// 	{
			// 		Title:       "Hello!",
			// 		Description: fmt.Sprintf("Hello, %s! How are you today?", userName),
			// 		Color:       0x00FF00, // Green color
			// 	},
			// },
			//
			// Components: []discordgo.MessageComponent{
			// 	discordgo.ActionsRow{
			// 		Components: []discordgo.MessageComponent{
			// 			&discordgo.Button{
			// 				Style:    discordgo.PrimaryButton,
			// 				Label:    "Click Me!",
			// 				CustomID: "hello_button_click",
			// 			},
			// 			&discordgo.Button{
			// 				Style:    discordgo.SecondaryButton,
			// 				Label:    "Don't Click Me!",
			// 				CustomID: "nothing_button_click",
			// 			},
			// 			&discordgo.Button{
			// 				Style:    discordgo.SecondaryButton,
			// 				Label:    "More",
			// 				CustomID: "more_button_click",
			// 			},
			// 		},
			// 	},

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

			// TODO add select box (with defaults already selected) for the capabilities
			Components: []discordgo.MessageComponent{
				// 					discordgo.Container{
				// 						AccentColor: &green, // Green color
				// 						Components: []discordgo.MessageComponent{
				// 							discordgo.TextDisplay{
				// 								Content: fmt.Sprintf("Chore created by **%s** with ID `%s`", chore.CreatorHandle, i.Interaction.ID),
				// 							},
				// 						},
				// 					},

				// 					discordgo.Container{
				// 						Components: []discordgo.MessageComponent{
				// 							discordgo.TextDisplay{
				// 								Content: fmt.Sprintf(`| Name                  | %s |
				// |-----------------------|----|
				// | Estimated Time (min)  | %d |`,
				// 									chore.Name, chore.EstimatedTimeMin),
				// 							},
				// 						},
				// 					},

				// discorgo.Embed

				// discordgo.Container{
				// 	Components: []discordgo.MessageComponent{
				// 		discordgo.TextDisplay{
				// 			Content: fmt.Sprintf("Chore created by **%s** with ID `%s`", chore.CreatorHandle, i.Interaction.ID),
				// 		},
				// 	},
				// },

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

				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.Button{
							Style:    discordgo.SuccessButton,
							Label:    "Schedule",
							CustomID: "schedule_button_click",
						},
						&discordgo.Button{
							Style:    discordgo.SecondaryButton,
							Label:    "Edit",
							CustomID: "edit_button_click",
						},
						&discordgo.Button{
							Style:    discordgo.DangerButton,
							Label:    "Delete",
							CustomID: "delete_button_click",
						},
					},
				},

				// TODO use this in the next interaction
				// discordgo.ActionsRow{
				// 	Components: []discordgo.MessageComponent{
				// 		&discordgo.Button{
				// 			Style:    discordgo.PrimaryButton,
				// 			Label:    "Ack",
				// 			CustomID: "ack_button_click",
				// 		},
				// 		&discordgo.Button{
				// 			Style:    discordgo.SecondaryButton,
				// 			Label:    "Reject",
				// 			CustomID: "reject_button_click",
				// 		},
				// 	},
				// },

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
		},
	})

	// Respond to the slash command interaction.
	// s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
	// 	Content: "Hell yeah!",
	// 	Flags:   discordgo.MessageFlagsEphemeral, // Makes the message visible only to the user who invoked the command.
	// })
}

func (st *Storage) Commands() error {

	// 2. Register a handler for incoming interactions (like slash commands).
	st.discord.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Interaction.ChannelID == "" {
			// If the interaction is not in a channel, we can't respond.
			// TODO
			// s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			// 	Type: discordgo.InteractionResponseChannelMessageWithSource,
			// 	Data: &discordgo.InteractionResponseData{
			// 		Content: "This command can only be used in a channel.",
			// 	},
			// })
			// return
		}

		if i.Interaction.Type == discordgo.InteractionApplicationCommand { // Ensure the interaction type is set correctly.
			// Check if the interaction is an ApplicationCommand (a slash command).
			switch i.ApplicationCommandData().Name {
			case "chore_create":
				choreCreate(st, s, i) // Call the appropriate handler function.
			}
		}
		if i.Type == discordgo.InteractionMessageComponent {
			data := i.MessageComponentData()
			switch data.CustomID {
			case "hello_button_click":
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseUpdateMessage,
					Data: &discordgo.InteractionResponseData{
						Content: "You clicked the button!",
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
				// s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				// 	Type: discordgo.InteractionResponseChannelMessageWithSource,
				// 	Data: &discordgo.InteractionResponseData{
				// 		Content: "You clicked the button!",
				// 		Flags:   discordgo.MessageFlagsEphemeral,
				// 	},
				// })
			case "hello_select_menu":
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: fmt.Sprintf("You selected: %s", data.Values),
						Flags:   discordgo.MessageFlagsEphemeral,
					},
				})
			case "nothing_button_click":
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseDeferredMessageUpdate,
				})
			case "more_button_click":
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseModal,
					Data: &discordgo.InteractionResponseData{
						CustomID: "more_modal",
						Title:    "More Information",
						Components: []discordgo.MessageComponent{
							discordgo.ActionsRow{
								Components: []discordgo.MessageComponent{
									&discordgo.TextInput{
										CustomID:    "more_text_input",
										Label:       "Enter more information",
										Style:       discordgo.TextInputShort,
										Placeholder: "Type something here...",
										Required:    true,
										MinLength:   1,
										MaxLength:   100,
									},
								},
							},
						},
					},
				})
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
	st.discord.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds

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

	skills, err := st.GetSkills()
	if err != nil {
		st.logger.Error("failed to get skills", "error", err)
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
					Name:        "estimated_time_min",
					Description: "The estimated time to complete the chore in minutes.",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "necessary_workers",
					Description: "The number of workers required to complete the chore.",
					Required:    false,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "assignment_timeout_min",
					Description: "The time in minutes after which the chore will be unassigned if not acked.",
					Required:    false,
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
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "capabilities",
					Description: "The capabilities required to complete the chore.",
					Required:    false,
					// TODO load from cabality roles
					Choices: skillsChoice,
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
		cmd, err := st.discord.ApplicationCommandCreate(st.discord.State.User.ID, st.conf.DiscordGuildId, v) // "" for global commands
		if err != nil {
			log.Fatalf("Cannot create '%v' command: %v", v.Name, err)
		}
		registeredCommands[i] = cmd
	}
	fmt.Println("Commands registered successfully!")

	// 6. Keep the bot running until an interrupt signal is received.
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc // Block until a signal is received.

	// 7. Cleanly close the Discord session.
	fmt.Println("Shutting down...")
	st.discord.Close()

	// 8. Unregister the commands when the bot shuts down.
	// This is good practice to avoid stale commands.
	fmt.Println("Unregistering commands...")
	for _, v := range registeredCommands {
		err := st.discord.ApplicationCommandDelete(st.discord.State.User.ID, st.conf.DiscordGuildId, v.ID) // "" for global commands
		if err != nil {
			log.Printf("Cannot delete '%v' command: %v", v.Name, err)
		}
	}
	fmt.Println("Commands unregistered.")
	return nil
}
