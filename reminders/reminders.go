package reminders

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

type Reminder struct {
	storage *storage.Storage
	ui      *ui.Ui
	chores  *chores.ChoresLogic
	logger  *slog.Logger
	conf    *Config
}

func NewReminder(storage *storage.Storage, ui *ui.Ui, chores *chores.ChoresLogic, logger *slog.Logger, conf *Config) *Reminder {
	return &Reminder{
		storage: storage,
		ui:      ui,
		chores:  chores,
		logger:  logger,
		conf:    conf,
	}
}

func (r *Reminder) CheckChores() {
	chores, err := r.storage.GetUnfinishedChores()
	if err != nil {
		r.logger.Error("Error getting unfinished chores", "error", err)
		return
	}

	for _, chore := range chores {
		if chore.Deadline != nil && chore.Deadline.Before(time.Now()) && !chore.AfterDeadlineReminded {
			r.ui.SendDM(chore.CreatorId, &discordgo.MessageSend{
				Content: fmt.Sprintf("Your chore `id: %d` is after its deadline %s.", chore.ID, r.ui.GetChoreMessageUrl(chore)),
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							&discordgo.Button{
								Style:    discordgo.SuccessButton,
								Label:    "Done!",
								CustomID: ui.DoneButtonClick + fmt.Sprint(chore.ID),
							},
							&discordgo.Button{
								Style:    discordgo.DangerButton,
								Label:    "Cancel",
								CustomID: ui.CancelButtonClick + fmt.Sprint(chore.ID),
							},
						},
					},
				},
			})
			chore.AfterDeadlineReminded = true
			_, err = r.storage.SaveChore(chore)
			if err != nil {
				r.logger.Error("Error saving chore", "error", err)
			}
		}

		ass, err := r.storage.GetChoreAssignments(chore.ID)
		if err != nil {
			r.logger.Error("Error getting chore assignments", "error", err)
			continue
		}

		for _, a := range ass {
			// Send DM that chore is after deadline.
			if chore.Deadline != nil && chore.Deadline.Before(time.Now()) && !a.AfterDeadlineReminded && a.Acked != nil {
				r.ui.SendDM(a.UserId, &discordgo.MessageSend{
					Content: fmt.Sprintf("Your assigned chore `id: %d` is after its deadline %s.", chore.ID, r.ui.GetChoreMessageUrl(chore)),
					Components: []discordgo.MessageComponent{
						discordgo.ActionsRow{
							Components: []discordgo.MessageComponent{
								&discordgo.Button{
									Style:    discordgo.SuccessButton,
									Label:    "Done!",
									CustomID: ui.DoneButtonClick + fmt.Sprint(chore.ID),
								},
							},
						},
					},
				})
				a.AfterDeadlineReminded = true
				// No need to remind twice.
				a.DeadlineReminded = true
				_, err = r.storage.SaveChoreAssignment(a)
				if err != nil {
					r.logger.Error("Error saving chore assignment", "error", err)
				}
			}

			// Send DM that chore is near deadline.
			if chore.Deadline != nil && !a.DeadlineReminded && a.Acked != nil {
				if time.Until(*chore.Deadline) < time.Duration(float64(chore.Deadline.Sub(chore.Created))*r.conf.ReminderRatio) {
					r.ui.SendDM(a.UserId, &discordgo.MessageSend{
						Content: fmt.Sprintf("Your assigned chore `id: %d` is nearing its deadline %s.", chore.ID, r.ui.GetChoreMessageUrl(chore)),
						Components: []discordgo.MessageComponent{
							discordgo.ActionsRow{
								Components: []discordgo.MessageComponent{
									&discordgo.Button{
										Style:    discordgo.SuccessButton,
										Label:    "Done!",
										CustomID: ui.DoneButtonClick + fmt.Sprint(chore.ID),
									},
								},
							},
						},
					})
					a.DeadlineReminded = true
					_, err = r.storage.SaveChoreAssignment(a)
					if err != nil {
						r.logger.Error("Error saving chore assignment", "error", err)
					}
				}
			}

			// Re-assignments disabled.
			if chore.AssignmentTimeoutMin == 0 {
				continue
			}

			if a.Acked != nil || a.Refused != nil || a.Timeouted != nil {
				continue
			}

			// reschedule expired assignment
			if time.Until(a.Created.Add(time.Duration(chore.AssignmentTimeoutMin)*time.Minute)) < 0 {
				r.ui.SendDM(a.UserId, &discordgo.MessageSend{
					Content: fmt.Sprintf("Your assignment for chore `id: %d` expired %s.", chore.ID, r.ui.GetChoreMessageUrl(chore)),
				})
				a.Timeout()
				_, err = r.storage.SaveChoreAssignment(a)
				if err != nil {
					r.logger.Error("Error saving chore assignment", "error", err)
				}
				users, err := r.storage.GetPresentUsers()
				if err != nil {
					r.logger.Error("Error getting present users", "error", err)
					return
				}
				_, err = r.chores.AssignChoresToUsers(users, chore)
				if err != nil {
					r.logger.Error("Error assigning chores to users", "error", err)
					return
				}
				r.ui.UpdateChoreMessage(chore)
				continue
			}

			// send DM that assignment is going to expire
			if !a.Reminded {
				if time.Until(a.Created.Add(time.Duration(chore.AssignmentTimeoutMin)*time.Minute)) < time.Duration(float64(time.Duration(chore.AssignmentTimeoutMin)*time.Minute)*r.conf.ReminderRatio) {
					r.ui.SendDM(a.UserId, &discordgo.MessageSend{
						Content: fmt.Sprintf("Your assignment for chore `id: %d` is about to expire. Please ack it %s.", chore.ID, r.ui.GetChoreMessageUrl(chore)),
					})
					a.Reminded = true
					_, err = r.storage.SaveChoreAssignment(a)
					if err != nil {
						r.logger.Error("Error saving chore assignment", "error", err)
					}
				}
			}

		}
	}
}

func (r *Reminder) RunReminder(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	for {
		timer := time.NewTimer(time.Duration(r.conf.CheckPeriodSeconds) * time.Second)
		select {
		case <-ctx.Done():
			r.logger.Debug("Reminder stopped: context cancelled", "reason", ctx.Err())
			return
		case <-timer.C:
			r.CheckChores()
		}
	}
}
