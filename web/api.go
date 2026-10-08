package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gdg-garage/garage-trip-chores/storage"
)

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
	writeJSON(rw, http.StatusOK, u)
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

func (w *Web) handleGetSkills(rw http.ResponseWriter, r *http.Request) {
	skills, err := w.storage.GetSkills()
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	if skills == nil {
		skills = []string{}
	}
	writeJSON(rw, http.StatusOK, map[string]any{"skills": skills})
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
	creatorID := in.CreatorId
	if creatorID == "" && u != nil {
		creatorID = u.DiscordId
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
		AssigneeId:           in.AssigneeId,
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

	saved, err := w.storage.SaveChore(chore)
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

	if in.DelayMin > 0 && !in.SelfReported {
		pubAt := now.Add(time.Duration(in.DelayMin) * time.Minute)
		_, _ = w.storage.CreateDelayedTask(saved.ID, pubAt, in.DelayMin)
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

	w.BroadcastWS(map[string]any{
		"type":  "task_claimed",
		"chore": view,
	})

	writeJSON(rw, http.StatusOK, view)
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
