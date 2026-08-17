package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type UserStatsDelta struct {
	Handle             string  `json:"handle"`
	UserId             string  `json:"user_id"`
	CurrentWorkedMin   float64 `json:"current_worked_min"`
	DeltaWorkedMin     float64 `json:"delta_worked_min"`
	CurrentWorkedCount float64 `json:"current_worked_count"`
	DeltaWorkedCount   float64 `json:"delta_worked_count"`
	CurrentAssignedMin float64 `json:"current_assigned_min"`
	CurrentNormalized  float64 `json:"current_normalized"`
	PresentTicks       int     `json:"present_ticks"`
}

type PromptData struct {
	StartTime    time.Time
	EndTime      time.Time
	Activity     *storage.TasksActivity
	CurrentStats map[string]storage.AggregatedUserStats
	PrevStats    map[string]storage.AggregatedUserStats
	UserHandles  map[string]string
}

func getUserName(userId string, handles map[string]string) string {
	if handle, ok := handles[userId]; ok && handle != "" {
		return handle
	}
	return userId
}

func CalculateStatsDeltas(currentStats, prevStats map[string]storage.AggregatedUserStats, handles map[string]string) []UserStatsDelta {
	allUsers := make(map[string]bool)
	for u := range currentStats {
		allUsers[u] = true
	}
	for u := range prevStats {
		allUsers[u] = true
	}

	var deltas []UserStatsDelta
	for u := range allUsers {
		curr := currentStats[u]
		prev := prevStats[u]

		deltaWorkedMin := curr.WorkedMin - prev.WorkedMin
		deltaWorkedCount := curr.WorkedCount - prev.WorkedCount

		deltas = append(deltas, UserStatsDelta{
			Handle:             getUserName(u, handles),
			UserId:             u,
			CurrentWorkedMin:   curr.WorkedMin,
			DeltaWorkedMin:     deltaWorkedMin,
			CurrentWorkedCount: curr.WorkedCount,
			DeltaWorkedCount:   deltaWorkedCount,
			CurrentAssignedMin: curr.AssignedMin,
			CurrentNormalized:  curr.NormalizedTotal,
			PresentTicks:       curr.PresentTicks,
		})
	}
	return deltas
}

func BuildPrompt(data PromptData) (string, error) {
	cetLoc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		cetLoc = time.FixedZone("CET", 1*3600)
	}

	startStr := data.StartTime.In(cetLoc).Format("2006-01-02 15:04 CET")
	endStr := data.EndTime.In(cetLoc).Format("2006-01-02 15:04 CET")

	deltas := CalculateStatsDeltas(data.CurrentStats, data.PrevStats, data.UserHandles)
	deltasJSON, _ := json.MarshalIndent(deltas, "", "  ")

	var completedSection strings.Builder
	if len(data.Activity.CompletedChores) > 0 {
		completedSection.WriteString("Completed Chores:\n")
		for _, c := range data.Activity.CompletedChores {
			workers := []string{}
			for _, wl := range data.Activity.WorkLogs {
				if wl.ChoreId == c.ID {
					workers = append(workers, fmt.Sprintf("%s (%d min)", getUserName(wl.UserId, data.UserHandles), wl.TimeSpentMin))
				}
			}
			workerStr := strings.Join(workers, ", ")
			if workerStr == "" {
				workerStr = "no specific worker logged"
			}
			completedSection.WriteString(fmt.Sprintf("- [Chore #%d] \"%s\" (Est: %d min) -> Completed by: %s\n", c.ID, c.Name, c.EstimatedTimeMin, workerStr))
		}
	} else {
		completedSection.WriteString("Completed Chores: None\n")
	}

	var createdSection strings.Builder
	if len(data.Activity.CreatedChores) > 0 {
		createdSection.WriteString("Newly Created Chores:\n")
		for _, c := range data.Activity.CreatedChores {
			creator := getUserName(c.CreatorId, data.UserHandles)
			caps := c.NecessaryCapabilities
			if caps == "" {
				caps = "none"
			}
			createdSection.WriteString(fmt.Sprintf("- [Chore #%d] \"%s\" (Created by: %s, Est: %d min, Skills needed: %s)\n", c.ID, c.Name, creator, c.EstimatedTimeMin, caps))
		}
	} else {
		createdSection.WriteString("Newly Created Chores: None\n")
	}

	var cancelledSection strings.Builder
	if len(data.Activity.CancelledChores) > 0 {
		cancelledSection.WriteString("Cancelled Chores:\n")
		for _, c := range data.Activity.CancelledChores {
			cancelledSection.WriteString(fmt.Sprintf("- [Chore #%d] \"%s\"\n", c.ID, c.Name))
		}
	}

	var assignmentsSection strings.Builder
	if len(data.Activity.UpdatedAssignments) > 0 {
		assignmentsSection.WriteString("Assignment Activity & Changes:\n")
		for _, a := range data.Activity.UpdatedAssignments {
			userName := getUserName(a.UserId, data.UserHandles)
			status := "Assigned (pending response)"
			if a.Acked != nil {
				status = "ACKed (accepted)"
			} else if a.Refused != nil {
				status = "REFUSED"
			} else if a.Timeouted != nil {
				status = "TIMED OUT (ignored the assignment!)"
			}
			choreName := a.Chore.Name
			if choreName == "" {
				choreName = fmt.Sprintf("Chore #%d", a.ChoreId)
			}
			assignmentsSection.WriteString(fmt.Sprintf("- %s on \"%s\": %s\n", userName, choreName, status))
		}
	}

	var idleSection strings.Builder
	if len(data.Activity.ActiveChores) > 0 {
		idleSection.WriteString("Currently Unfinished Open Chores:\n")
		for _, c := range data.Activity.ActiveChores {
			idleSection.WriteString(fmt.Sprintf("- [Chore #%d] \"%s\" (Est: %d min)\n", c.ID, c.Name, c.EstimatedTimeMin))
		}
	}

	prompt := fmt.Sprintf(`You are the official, witty, humorous, and slightly sassy chores digest commentator for the GDG Garage Trip!

Task:
Summarize what happened during the period %s to %s.
Your audience is a group of developers, hackers, and friends on a team trip.

Here is what happened since the previous run:

%s
%s
%s
%s
%s

Machine-Readable User Statistics (Current totals & changes since previous run):
%s

Instructions for your summary:
1. **Highlight Accomplishments**: Give shout-outs to people who completed tasks and worked hard.
2. **Roast / Call Out Slacking**: Lightheartedly tease anyone who refused tasks, timed out on assignments, or was assigned and did nothing while others worked.
3. **Leaderboard / Stats Shift**: Briefly mention who made moves on the leaderboard (who gained the most minutes, who is carrying the team, who has 0 minutes).
4. **Style & Tone**:
   - Keep it **quite brief** (around 2 to 4 concise paragraphs or bullet points).
   - Must be **definitely funny, punchy, and entertaining**.
   - Format with Discord markdown (**bold**, bullet points, emojis).
   - Do NOT use giant header tags like "# " or "## "; use bolding like "**Title**" instead.
   - Do NOT invent fake users; only mention the users and chore names provided above.
   - Keep the entire response under 1500 characters so it fits neatly in a single Discord message.`,
		startStr, endStr,
		completedSection.String(),
		createdSection.String(),
		cancelledSection.String(),
		assignmentsSection.String(),
		idleSection.String(),
		string(deltasJSON),
	)

	return prompt, nil
}
