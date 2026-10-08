package web

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

var funnyAckMessages = []string{
	"🦸 Chore hero incoming! Thanks, %s!",
	"🎉 %s said yes to the mess!",
	"🧹 %s is on it like a bonnet!",
	"💪 Absolute legend, %s. The dishes tremble.",
	"🚀 %s launched into action!",
	"🏆 Garage Trip MVP: %s!",
	"🔥 %s grabbed it before anyone else could blink.",
	"🧽 Scrub-a-dub, %s to the rescue!",
	"🥇 %s just earned some serious chore cred.",
	"😎 Cool, calm, and cleaning: that's %s.",
	"🎯 %s claimed it. Bullseye.",
	"🙌 The mountain thanks you, %s!",
	"⚡ Lightning-fast %s strikes again.",
	"🐝 Busy as a bee, %s buzzes off to work.",
	"🎈 Party's over, chore's on — go %s!",
}

func getFunnyAck(name string) string {
	if name == "" {
		name = "friend"
	}
	idx := rand.IntN(len(funnyAckMessages))
	return fmt.Sprintf(funnyAckMessages[idx], name)
}

func writeJSON(rw http.ResponseWriter, status int, data any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(data)
}

func writeError(rw http.ResponseWriter, status int, msg string) {
	writeJSON(rw, status, map[string]string{"detail": msg})
}

func (w *Web) handleGetMe(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Not logged in")
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{
		"discord_id": u.DiscordId,
		"name":       u.Name,
		"handle":     u.Handle,
		"profile": map[string]any{
			"name":           u.Name,
			"discord_handle": u.Handle,
		},
		"capabilities": u.Capabilities,
		"is_present":   u.IsPresent,
		"is_admin":     u.IsAdmin,
	})
}

func (w *Web) handleGetManualWork(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Not logged in")
		return
	}

	worklogs, _ := w.storage.GetWorkLogs()
	var userLogs []map[string]any
	for _, wl := range worklogs {
		if wl.UserId == u.DiscordId && wl.SelfReported {
			userLogs = append(userLogs, map[string]any{
				"id":             wl.ID,
				"chore_id":       wl.ChoreId,
				"description":    wl.Chore.Name,
				"time_spent_min": wl.TimeSpentMin,
				"created":        wl.Chore.Created,
			})
		}
	}
	writeJSON(rw, http.StatusOK, userLogs)
}

func (w *Web) handlePostManualWork(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Not logged in")
		return
	}

	var in ManualWorkIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if in.Description == "" {
		writeError(rw, http.StatusBadRequest, "Description required")
		return
	}
	if in.Minutes == 0 {
		in.Minutes = 15
	}

	now := time.Now()
	name := in.Description
	if !strings.HasPrefix(name, "📝") {
		name = "📝 " + name
	}

	chore := storage.Chore{
		Name:                 name,
		EstimatedTimeMin:     in.Minutes,
		NecessaryWorkers:     1,
		AssignmentTimeoutMin: 15,
		CreatorId:            u.DiscordId,
		Created:              now,
		Completed:            &now,
		SelfReported:         true,
	}

	saved, err := w.storage.SaveChore(chore)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, "Failed to create chore")
		return
	}

	wl := storage.WorkLog{
		UserId:       u.DiscordId,
		ChoreId:      saved.ID,
		TimeSpentMin: in.Minutes,
		SelfReported: true,
	}
	_, _ = w.storage.SaveWorkLog(wl)

	w.BroadcastWS(map[string]any{
		"type": "workload_updated",
	})

	writeJSON(rw, http.StatusOK, map[string]any{"ok": true, "id": saved.ID})
}

func (w *Web) handleGetTemplates(rw http.ResponseWriter, r *http.Request) {
	tpls, err := w.storage.GetTemplates()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	var res []map[string]any
	for _, t := range tpls {
		var caps []string
		_ = json.Unmarshal([]byte(t.NecessaryCapabilities), &caps)
		res = append(res, map[string]any{
			"key":                    t.Key,
			"name":                   t.Name,
			"necessary_workers":      t.NecessaryWorkers,
			"estimated_time_min":     t.EstimatedTimeMin,
			"assignment_timeout_min": t.AssignmentTimeoutMin,
			"necessary_capabilities": caps,
			"scales_with_headcount":  t.ScalesWithHeadcount,
			"per_person_min":         t.PerPersonMin,
			"sort_order":             t.SortOrder,
		})
	}
	writeJSON(rw, http.StatusOK, map[string]any{"templates": res})
}

func (w *Web) handlePostTemplate(rw http.ResponseWriter, r *http.Request) {
	var in TemplateIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if in.Name == "" {
		writeError(rw, http.StatusBadRequest, "Name required")
		return
	}

	key := strings.ToLower(strings.ReplaceAll(in.Name, " ", "-"))
	capsJSON, _ := json.Marshal(in.NecessaryCapabilities)
	t := storage.ChoreTemplate{
		Key:                   key,
		Name:                  in.Name,
		NecessaryWorkers:      in.NecessaryWorkers,
		EstimatedTimeMin:      in.EstimatedTimeMin,
		AssignmentTimeoutMin:  in.AssignmentTimeoutMin,
		NecessaryCapabilities: string(capsJSON),
		ScalesWithHeadcount:   in.ScalesWithHeadcount,
		PerPersonMin:          in.PerPersonMin,
	}
	if err := w.storage.UpsertTemplate(&t); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, t)
}

func (w *Web) handlePutTemplate(rw http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var in TemplateIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}

	capsJSON, _ := json.Marshal(in.NecessaryCapabilities)
	t := storage.ChoreTemplate{
		Key:                   key,
		Name:                  in.Name,
		NecessaryWorkers:      in.NecessaryWorkers,
		EstimatedTimeMin:      in.EstimatedTimeMin,
		AssignmentTimeoutMin:  in.AssignmentTimeoutMin,
		NecessaryCapabilities: string(capsJSON),
		ScalesWithHeadcount:   in.ScalesWithHeadcount,
		PerPersonMin:          in.PerPersonMin,
	}
	if err := w.storage.UpsertTemplate(&t); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, t)
}

func (w *Web) handleDeleteTemplate(rw http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if err := w.storage.DeleteTemplate(key); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{"ok": true})
}

func (w *Web) buildScheduledTaskView(t storage.ScheduledTask) ScheduledTaskView {
	creatorName := t.CreatorName
	if creatorName == "" {
		creatorName = w.storage.ResolveUserName(t.CreatorId)
	}
	return ScheduledTaskView{
		ID:                    t.ID,
		Name:                  t.Name,
		Description:           t.Description,
		CronExpr:              t.CronExpr,
		NecessaryWorkers:      t.NecessaryWorkers,
		EstimatedTimeMin:      t.EstimatedTimeMin,
		AssignmentTimeoutMin:  t.AssignmentTimeoutMin,
		NecessaryCapabilities: t.GetCapabilities(),
		CreatorId:             t.CreatorId,
		CreatorName:           creatorName,
		Enabled:               t.Enabled,
		TemplateKey:           t.TemplateKey,
		LastRunAt:             t.LastRunAt,
		NextRunAt:             t.NextRunAt,
		CreatedAt:             t.CreatedAt,
	}
}

func (w *Web) handleGetSchedules(rw http.ResponseWriter, r *http.Request) {
	tasks, err := w.storage.GetScheduledTasks()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	var views []ScheduledTaskView
	for _, t := range tasks {
		views = append(views, w.buildScheduledTaskView(t))
	}
	if views == nil {
		views = []ScheduledTaskView{}
	}
	writeJSON(rw, http.StatusOK, map[string]any{"schedules": views})
}

func (w *Web) handlePostSchedule(rw http.ResponseWriter, r *http.Request) {
	var in ScheduledTaskIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeError(rw, http.StatusBadRequest, "Name required")
		return
	}
	if strings.TrimSpace(in.CronExpr) == "" {
		writeError(rw, http.StatusBadRequest, "Cron expression required")
		return
	}

	nextRun, err := ui.ParseCronNext(in.CronExpr, time.Now(), nil)
	if err != nil {
		writeError(rw, http.StatusBadRequest, fmt.Sprintf("Invalid cron expression: %v", err))
		return
	}

	u := w.GetCurrentUser(r)
	creatorID := strings.TrimSpace(in.CreatorId)
	if u != nil {
		if creatorID == "" || !u.IsAdmin {
			creatorID = u.DiscordId
		}
	}
	if creatorID == "" {
		creatorID = "tablet"
	}
	creatorName := ""
	if u != nil && creatorID == u.DiscordId {
		creatorName = u.Name
	} else {
		creatorName = w.storage.ResolveUserName(creatorID)
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	capsJSON, _ := json.Marshal(in.NecessaryCapabilities)
	workers := in.NecessaryWorkers
	if workers == 0 {
		workers = 1
	}
	est := in.EstimatedTimeMin
	if est == 0 {
		est = 10
	}
	timeout := in.AssignmentTimeoutMin
	if timeout == 0 {
		timeout = 15
	}

	task := storage.ScheduledTask{
		Name:                  in.Name,
		Description:           in.Description,
		CronExpr:              in.CronExpr,
		NecessaryWorkers:      workers,
		EstimatedTimeMin:      est,
		AssignmentTimeoutMin:  timeout,
		NecessaryCapabilities: string(capsJSON),
		CreatorId:             creatorID,
		CreatorName:           creatorName,
		Enabled:               enabled,
		TemplateKey:           in.TemplateKey,
		NextRunAt:             nextRun,
	}

	created, err := w.storage.CreateScheduledTask(task)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, w.buildScheduledTaskView(created))
}

func (w *Web) handleGetSchedule(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	task, err := w.storage.GetScheduledTask(uint(id))
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		writeError(rw, http.StatusNotFound, "Schedule not found")
		return
	}
	writeJSON(rw, http.StatusOK, w.buildScheduledTaskView(*task))
}

func (w *Web) handlePutSchedule(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	existing, err := w.storage.GetScheduledTask(uint(id))
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if existing == nil {
		writeError(rw, http.StatusNotFound, "Schedule not found")
		return
	}

	var in ScheduledTaskIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if strings.TrimSpace(in.Name) != "" {
		existing.Name = in.Name
	}
	existing.Description = in.Description
	if strings.TrimSpace(in.CronExpr) != "" {
		nextRun, err := ui.ParseCronNext(in.CronExpr, time.Now(), nil)
		if err != nil {
			writeError(rw, http.StatusBadRequest, fmt.Sprintf("Invalid cron expression: %v", err))
			return
		}
		existing.CronExpr = in.CronExpr
		existing.NextRunAt = nextRun
	}
	if in.NecessaryWorkers > 0 {
		existing.NecessaryWorkers = in.NecessaryWorkers
	}
	if in.EstimatedTimeMin > 0 {
		existing.EstimatedTimeMin = in.EstimatedTimeMin
	}
	if in.AssignmentTimeoutMin > 0 {
		existing.AssignmentTimeoutMin = in.AssignmentTimeoutMin
	}
	if in.NecessaryCapabilities != nil {
		capsJSON, _ := json.Marshal(in.NecessaryCapabilities)
		existing.NecessaryCapabilities = string(capsJSON)
	}
	if in.CreatorId != "" {
		existing.CreatorId = in.CreatorId
		existing.CreatorName = w.storage.ResolveUserName(in.CreatorId)
	}
	if in.Enabled != nil {
		existing.Enabled = *in.Enabled
		if !existing.Enabled {
			existing.NextRunAt = nil
		} else if existing.NextRunAt == nil {
			nextRun, _ := ui.ParseCronNext(existing.CronExpr, time.Now(), nil)
			existing.NextRunAt = nextRun
		}
	}
	existing.TemplateKey = in.TemplateKey

	if err := w.storage.UpdateScheduledTask(*existing); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, w.buildScheduledTaskView(*existing))
}

func (w *Web) handleDeleteSchedule(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	if err := w.storage.DeleteScheduledTask(uint(id)); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{"ok": true})
}

func (w *Web) handleToggleSchedule(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	task, err := w.storage.GetScheduledTask(uint(id))
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		writeError(rw, http.StatusNotFound, "Schedule not found")
		return
	}

	task.Enabled = !task.Enabled
	if task.Enabled {
		nextRun, _ := ui.ParseCronNext(task.CronExpr, time.Now(), nil)
		task.NextRunAt = nextRun
	} else {
		task.NextRunAt = nil
	}
	if err := w.storage.UpdateScheduledTask(*task); err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, w.buildScheduledTaskView(*task))
}

func (w *Web) handleRunSchedule(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid ID")
		return
	}

	if w.ui == nil {
		writeError(rw, http.StatusInternalServerError, "UI service not available")
		return
	}

	chore, err := w.ui.ExecuteScheduledTask(uint(id))
	if err != nil {
		writeError(rw, http.StatusInternalServerError, fmt.Sprintf("Failed to run schedule: %v", err))
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{"ok": true, "chore_id": chore.ID})
}


func (w *Web) handleGetSkills(rw http.ResponseWriter, r *http.Request) {
	skills, err := w.storage.GetSkills()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if skills == nil {
		skills = []string{}
	}
	writeJSON(rw, http.StatusOK, skills)
}

func (w *Web) handleGetStats(rw http.ResponseWriter, r *http.Request) {
	stats, err := w.storage.GetAggregatedStats()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{"stats": stats})
}

func (w *Web) handleGetUsers(rw http.ResponseWriter, r *http.Request) {
	dir := w.BuildPersonDirectory()
	var list []UserInfo
	for _, u := range dir {
		list = append(list, u)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	writeJSON(rw, http.StatusOK, map[string]any{
		"users":          list,
		"children_count": w.conf.ChildrenCount,
	})
}

func (w *Web) handleGetPeople(rw http.ResponseWriter, r *http.Request) {
	pool := w.GetPeoplePool()
	writeJSON(rw, http.StatusOK, map[string]any{
		"people":         pool,
		"children_count": w.conf.ChildrenCount,
	})
}

func (w *Web) handleGetLeaderboard(rw http.ResponseWriter, r *http.Request) {
	writeJSON(rw, http.StatusOK, map[string]any{"rows": w.Leaderboard()})
}

func (w *Web) handleGetUserDetail(rw http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	userID = strings.TrimPrefix(userID, "/")
	writeJSON(rw, http.StatusOK, w.UserDetail(userID))
}

func (w *Web) handleGetChores(rw http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"
	views, err := w.ListChoreViews(activeOnly)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, map[string]any{"chores": views})
}

func (w *Web) handleGetChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	delayed, _ := w.storage.GetDelayedTaskForChore(uint(id))

	view := w.BuildChoreView(chore, worklogs, assignments, delayed, dir)
	writeJSON(rw, http.StatusOK, view)
}

func (w *Web) handleGetSuggestions(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	writeJSON(rw, http.StatusOK, w.SuggestionsFor(chore))
}

func (w *Web) handlePostChore(rw http.ResponseWriter, r *http.Request) {
	var in ChoreCreateIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}
	if in.Name == "" {
		writeError(rw, http.StatusBadRequest, "Name required")
		return
	}

	u := w.GetCurrentUser(r)
	creatorID := strings.TrimSpace(in.CreatorId)
	if u != nil {
		// When authenticated via Discord session, use logged-in user's ID
		// unless an admin or tablet manager explicitly specified another attendee
		if creatorID == "" || !u.IsAdmin {
			creatorID = u.DiscordId
		}
	}

	if creatorID == "" {
		if in.SelfReported {
			writeError(rw, http.StatusBadRequest, "User ID is required to log a self-reported chore")
			return
		}
		creatorID = "tablet"
	}

	workers := in.NecessaryWorkers
	if workers == 0 {
		workers = 1
	}
	est := in.EstimatedTimeMin
	if est == 0 {
		est = 10
	}
	timeout := in.AssignmentTimeoutMin
	if timeout == 0 {
		timeout = 15
	}

	now := time.Now()
	deadline := now.Add(24 * time.Hour)

	chore := storage.Chore{
		Name:                 in.Name,
		EstimatedTimeMin:     est,
		NecessaryWorkers:     workers,
		AssignmentTimeoutMin: timeout,
		CreatorId:            creatorID,
		DelayMin:             in.DelayMin,
		SelfReported:         in.SelfReported,
		TemplateKey:          in.TemplateKey,
		Created:              now,
		Deadline:             &deadline,
	}
	chore.SetCapabilities(in.NecessaryCapabilities)

	if in.SelfReported {
		chore.Completed = &now
	}

	var saved storage.Chore
	var err error
	if in.DelayMin > 0 {
		chore.Draft = false
		saved, err = w.storage.SaveChore(chore)
		if err != nil {
			writeError(rw, http.StatusInternalServerError, err.Error())
			return
		}
		pubAt := now.Add(time.Duration(in.DelayMin) * time.Minute)
		_, _ = w.storage.CreateDelayedTask(saved.ID, pubAt, in.DelayMin)
	} else if w.ui != nil {
		saved, _, err = w.ui.PublishChore(chore)
		if err != nil {
			w.logger.Warn("Failed to publish chore via UI server", "error", err)
			saved, err = w.storage.SaveChore(chore)
			if err != nil {
				writeError(rw, http.StatusInternalServerError, err.Error())
				return
			}
			if in.SelfReported {
				wl := storage.WorkLog{
					UserId:       creatorID,
					ChoreId:      saved.ID,
					TimeSpentMin: est,
					SelfReported: true,
				}
				_, _ = w.storage.SaveWorkLog(wl)
			}
		}
	} else {
		saved, err = w.storage.SaveChore(chore)
		if err != nil {
			writeError(rw, http.StatusInternalServerError, err.Error())
			return
		}
		if in.SelfReported {
			wl := storage.WorkLog{
				UserId:       creatorID,
				ChoreId:      saved.ID,
				TimeSpentMin: est,
				SelfReported: true,
			}
			_, _ = w.storage.SaveWorkLog(wl)
		}
	}

	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(saved.ID)
	delayed, _ := w.storage.GetDelayedTaskForChore(saved.ID)
	view := w.BuildChoreView(saved, worklogs, nil, delayed, dir)
	sug := w.SuggestionsFor(saved)

	w.BroadcastWS(map[string]any{
		"type":        "task_created",
		"chore":       view,
		"suggestions": sug.Top,
	})

	writeJSON(rw, http.StatusOK, view)
}

func (w *Web) handleClaimChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Not logged in")
		return
	}

	now := time.Now()
	// Find or create assignment
	as, _ := w.storage.GetChoreAssignments(uint(id))
	found := false
	for _, a := range as {
		if a.UserId == u.DiscordId {
			found = true
			a.Acked = &now
			_, _ = w.storage.SaveChoreAssignment(a)
			break
		}
	}

	if !found {
		newA := storage.ChoreAssignment{
			UserId:      u.DiscordId,
			ChoreId:     uint(id),
			Created:     now,
			Acked:       &now,
			Volunteered: true,
		}
		_, _ = w.storage.SaveChoreAssignment(newA)
	}

	chore, _ := w.storage.GetChore(uint(id))
	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	view := w.BuildChoreView(chore, worklogs, assignments, nil, dir)

	sug := w.SuggestionsFor(chore)
	w.BroadcastWS(map[string]any{
		"type":        "task_claimed",
		"chore":       view,
		"by":          u.DiscordId,
		"suggestions": sug.Top,
	})

	name := u.Name
	if name == "" {
		name = u.Handle
	}
	ack := getFunnyAck(name)

	writeJSON(rw, http.StatusOK, map[string]any{
		"ack":   ack,
		"chore": view,
	})
}

func (w *Web) handleUnclaimChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Not logged in")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	if w.ui != nil {
		_, _ = w.ui.RejectChore(uint(id), u.DiscordId)
	} else {
		ass, aErr := w.storage.GetChoreAssignment(uint(id), u.DiscordId)
		if aErr == nil {
			ass.Refuse()
			_, _ = w.storage.SaveChoreAssignment(ass)
		}
	}

	chore, _ = w.storage.GetChore(uint(id))
	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	view := w.BuildChoreView(chore, worklogs, assignments, nil, dir)

	sug := w.SuggestionsFor(chore)
	w.BroadcastWS(map[string]any{
		"type":        "task_claimed",
		"chore":       view,
		"by":          u.DiscordId,
		"suggestions": sug.Top,
	})

	writeJSON(rw, http.StatusOK, map[string]any{
		"ack":   "Dropped chore. Back on the board! 🧹",
		"chore": view,
	})
}

type AssignIn struct {
	DiscordId string `json:"discord_id"`
}

func (w *Web) handleAssignChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	var in AssignIn
	_ = json.NewDecoder(r.Body).Decode(&in)

	assigneeId := strings.TrimSpace(in.DiscordId)
	if assigneeId == "" {
		sug := w.SuggestionsFor(chore)
		if len(sug.Top) == 0 {
			writeError(rw, http.StatusConflict, "No eligible person available to auto-assign.")
			return
		}
		assigneeId = sug.Top[0]
	}

	now := time.Now()
	as, _ := w.storage.GetChoreAssignments(uint(id))
	found := false
	for _, a := range as {
		if a.UserId == assigneeId {
			found = true
			a.Acked = &now
			_, _ = w.storage.SaveChoreAssignment(a)
			break
		}
	}
	if !found {
		newA := storage.ChoreAssignment{
			UserId:      assigneeId,
			ChoreId:     uint(id),
			Created:     now,
			Acked:       &now,
			Volunteered: false,
		}
		_, _ = w.storage.SaveChoreAssignment(newA)
	}

	chore, _ = w.storage.GetChore(uint(id))
	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	view := w.BuildChoreView(chore, worklogs, assignments, nil, dir)

	name := assigneeId
	if u, ok := dir[assigneeId]; ok && u.Name != "" {
		name = u.Name
	}

	sug := w.SuggestionsFor(chore)
	w.BroadcastWS(map[string]any{
		"type":        "task_claimed",
		"chore":       view,
		"by":          assigneeId,
		"suggestions": sug.Top,
	})

	writeJSON(rw, http.StatusOK, map[string]any{
		"chore":    view,
		"assigned": map[string]string{"discord_id": assigneeId, "name": name},
		"ack":      fmt.Sprintf("Assigned to %s ✓", name),
	})
}

func (w *Web) handleUnassignChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	var in AssignIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.DiscordId) == "" {
		writeError(rw, http.StatusBadRequest, "discord_id is required")
		return
	}
	targetId := strings.TrimSpace(in.DiscordId)

	if w.ui != nil {
		_, _ = w.ui.RejectChore(uint(id), targetId)
	} else {
		ass, aErr := w.storage.GetChoreAssignment(uint(id), targetId)
		if aErr == nil {
			ass.Refuse()
			_, _ = w.storage.SaveChoreAssignment(ass)
		}
	}

	chore, _ := w.storage.GetChore(uint(id))
	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	view := w.BuildChoreView(chore, worklogs, assignments, nil, dir)

	name := targetId
	if u, ok := dir[targetId]; ok && u.Name != "" {
		name = u.Name
	}

	sug := w.SuggestionsFor(chore)
	w.BroadcastWS(map[string]any{
		"type":        "task_claimed",
		"chore":       view,
		"by":          targetId,
		"suggestions": sug.Top,
	})

	writeJSON(rw, http.StatusOK, map[string]any{
		"chore": view,
		"ack":   fmt.Sprintf("Unassigned %s ✓", name),
	})
}

type ChoreTimeIn struct {
	DiscordId    string `json:"discord_id"`
	TimeSpentMin uint   `json:"time_spent_min"`
}

func (w *Web) handleChoreTime(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	var in ChoreTimeIn
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid JSON")
		return
	}

	targetUID := strings.TrimSpace(in.DiscordId)
	if targetUID == "" {
		u := w.GetCurrentUser(r)
		if u == nil {
			writeError(rw, http.StatusBadRequest, "discord_id required")
			return
		}
		targetUID = u.DiscordId
	}

	logs, _ := w.storage.GetWorkLogsForChore(uint(id))
	var existing *storage.WorkLog
	for _, l := range logs {
		if l.UserId == targetUID {
			existing = &l
			break
		}
	}

	if existing != nil {
		existing.TimeSpentMin = in.TimeSpentMin
		_, err = w.storage.SaveWorkLog(*existing)
	} else {
		newLog := storage.WorkLog{
			UserId:       targetUID,
			ChoreId:      uint(id),
			TimeSpentMin: in.TimeSpentMin,
			SelfReported: true,
		}
		_, err = w.storage.SaveWorkLog(newLog)
	}

	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	chore, _ := w.storage.GetChore(uint(id))
	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	view := w.BuildChoreView(chore, worklogs, assignments, nil, dir)

	w.BroadcastWS(map[string]any{
		"type":  "workload_updated",
		"chore": view,
	})

	writeJSON(rw, http.StatusOK, map[string]any{
		"chore": view,
		"ack":   "Time updated ✓",
	})
}

func (w *Web) handleDoneChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	now := time.Now()
	chore.Completed = &now
	saved, err := w.storage.SaveChore(chore)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	// Ensure worklog exists for claimers
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	existingWLUsers := make(map[string]bool)
	for _, wl := range worklogs {
		existingWLUsers[wl.UserId] = true
	}

	for _, a := range assignments {
		if a.Acked != nil && !existingWLUsers[a.UserId] {
			wl := storage.WorkLog{
				UserId:       a.UserId,
				ChoreId:      uint(id),
				TimeSpentMin: chore.EstimatedTimeMin,
				SelfReported: false,
			}
			_, _ = w.storage.SaveWorkLog(wl)
		}
	}

	// If no claimers had worklogs and current user logged it done
	u := w.GetCurrentUser(r)
	if len(assignments) == 0 && u != nil && !existingWLUsers[u.DiscordId] {
		wl := storage.WorkLog{
			UserId:       u.DiscordId,
			ChoreId:      uint(id),
			TimeSpentMin: chore.EstimatedTimeMin,
			SelfReported: false,
		}
		_, _ = w.storage.SaveWorkLog(wl)
	}

	dir := w.BuildPersonDirectory()
	worklogs, _ = w.storage.GetWorkLogsForChore(uint(id))
	view := w.BuildChoreView(saved, worklogs, assignments, nil, dir)

	w.BroadcastWS(map[string]any{
		"type":  "task_done",
		"chore": map[string]any{"id": id},
	})
	w.BroadcastWS(map[string]any{
		"type": "workload_updated",
	})

	writeJSON(rw, http.StatusOK, view)
}

func (w *Web) handleHelpChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	u := w.GetCurrentUser(r)
	if u == nil {
		writeError(rw, http.StatusUnauthorized, "Not logged in")
		return
	}

	var in struct {
		TimeSpentMin *uint `json:"time_spent_min"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)

	mins := chore.EstimatedTimeMin
	if in.TimeSpentMin != nil && *in.TimeSpentMin > 0 {
		mins = *in.TimeSpentMin
	}

	wl := storage.WorkLog{
		UserId:       u.DiscordId,
		ChoreId:      uint(id),
		TimeSpentMin: mins,
		SelfReported: false,
	}
	_, err = w.storage.SaveWorkLog(wl)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	dir := w.BuildPersonDirectory()
	worklogs, _ := w.storage.GetWorkLogsForChore(uint(id))
	assignments, _ := w.storage.GetChoreAssignments(uint(id))
	view := w.BuildChoreView(chore, worklogs, assignments, nil, dir)

	w.BroadcastWS(map[string]any{
		"type":  "task_updated",
		"chore": view,
	})
	w.BroadcastWS(map[string]any{
		"type": "workload_updated",
	})

	writeJSON(rw, http.StatusOK, view)
}

func (w *Web) handleDeleteChore(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeError(rw, http.StatusBadRequest, "Invalid chore ID")
		return
	}

	chore, err := w.storage.GetChore(uint(id))
	if err != nil {
		writeError(rw, http.StatusNotFound, "Chore not found")
		return
	}

	now := time.Now()
	chore.Cancelled = &now
	_, _ = w.storage.SaveChore(chore)
	_ = w.storage.CancelDelayedTaskByChoreId(uint(id))

	w.BroadcastWS(map[string]any{
		"type":  "task_done",
		"chore": map[string]any{"id": id},
	})

	writeJSON(rw, http.StatusOK, map[string]any{"ok": true})
}

func (w *Web) handleSummary(rw http.ResponseWriter, r *http.Request) {
	if w.summarizer == nil {
		writeError(rw, http.StatusInternalServerError, "LLM summarizer not configured")
		return
	}
	if err := w.summarizer.RunOnce(r.Context()); err != nil {
		writeError(rw, http.StatusInternalServerError, fmt.Sprintf("Failed to run summary: %v", err))
		return
	}
	writeJSON(rw, http.StatusOK, map[string]string{"message": "LLM summary triggered and published successfully"})
}
