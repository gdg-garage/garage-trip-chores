package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

func setupTestWeb(t *testing.T) (*Web, chi.Router) {
	tmpDb, err := os.CreateTemp("", "web_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb.Close()
	t.Cleanup(func() { os.Remove(tmpDb.Name()) })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := storage.New(storage.Config{
		DbPath: tmpDb.Name(),
	}, logger)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	choresLogic := chores.NewChoresLogic(st, logger, chores.Config{})
	uiInstance := ui.NewUi(st, logger, &choresLogic, nil, ui.Config{})
	webInstance, err := New(st, logger, &choresLogic, uiInstance, Config{
		AuthRequired: false,
	})
	if err != nil {
		t.Fatalf("failed to init web: %v", err)
	}

	router := chi.NewRouter()
	webInstance.RegisterRoutes(router)

	return webInstance, router
}

func TestWebPagesAndAPI(t *testing.T) {
	_, router := setupTestWeb(t)

	// 1. Test GET /feed
	req := httptest.NewRequest(http.MethodGet, "/feed", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /feed, got %d", rr.Code)
	}

	// 2. Test GET /api/templates
	req = httptest.NewRequest(http.MethodGet, "/api/templates", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/templates, got %d", rr.Code)
	}
	var tplsResp struct {
		Templates []map[string]any `json:"templates"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&tplsResp); err != nil {
		t.Fatalf("failed to decode templates: %v", err)
	}
	if len(tplsResp.Templates) == 0 {
		t.Fatalf("expected seeded templates, got 0")
	}

	// 3. Test POST /api/chores (create standard chore)
	choreIn := ChoreCreateIn{
		Name:             "Wash dishes 🌶️",
		EstimatedTimeMin: 15,
		NecessaryWorkers: 1,
		DelayMin:         0,
		SelfReported:     false,
	}
	body, _ := json.Marshal(choreIn)
	req = httptest.NewRequest(http.MethodPost, "/api/chores", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST /api/chores, got %d: %s", rr.Code, rr.Body.String())
	}
	var created ChoreView
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created chore: %v", err)
	}
	if created.ID == 0 || !created.Urgent || created.Spiciness != 1 {
		t.Fatalf("unexpected created chore: %+v", created)
	}

	// 4. Test GET /api/chores
	req = httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/chores, got %d", rr.Code)
	}
	var choresList struct {
		Chores []ChoreView `json:"chores"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&choresList); err != nil {
		t.Fatalf("failed to decode chores list: %v", err)
	}
	if len(choresList.Chores) != 1 {
		t.Fatalf("expected 1 chore, got %d", len(choresList.Chores))
	}

	// 5. Test POST /api/chores (create self-reported chore)
	srIn := ChoreCreateIn{
		Name:             "Clean sauna",
		EstimatedTimeMin: 20,
		SelfReported:     true,
		CreatorId:        "user123",
	}
	body, _ = json.Marshal(srIn)
	req = httptest.NewRequest(http.MethodPost, "/api/chores", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for self-reported chore, got %d: %s", rr.Code, rr.Body.String())
	}
	var srChore ChoreView
	if err := json.NewDecoder(rr.Body).Decode(&srChore); err != nil {
		t.Fatalf("failed to decode self-reported chore: %v", err)
	}
	if !srChore.SelfReported || srChore.Completed == nil || srChore.WorkedMinTotal != 20 {
		t.Fatalf("unexpected self-reported chore: %+v", srChore)
	}

	// 6. Test GET /api/users (should return seeded attendees from discord user map)
	req = httptest.NewRequest(http.MethodGet, "/api/users", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/users, got %d", rr.Code)
	}
	var usersResp struct {
		Users         []UserInfo `json:"users"`
		ChildrenCount int        `json:"children_count"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&usersResp); err != nil {
		t.Fatalf("failed to decode users response: %v", err)
	}
	if len(usersResp.Users) == 0 {
		t.Fatalf("expected users from seeded user map, got 0")
	}

	// Verify a known user translation (e.g. 378532044558303233 -> Dongalis (Dominik N.))
	foundDongalis := false
	for _, u := range usersResp.Users {
		if u.DiscordId == "378532044558303233" {
			foundDongalis = true
			if u.Name != "Dongalis (Dominik N.)" {
				t.Fatalf("expected translated name 'Dongalis (Dominik N.)', got '%s'", u.Name)
			}
			if u.Handle != "dongalis" {
				t.Fatalf("expected handle 'dongalis', got '%s'", u.Handle)
			}
		}
	}
	if !foundDongalis {
		t.Fatalf("expected to find Dongalis in users list")
	}

	// 7. Test GET /api/leaderboard (wrap into rows)
	req = httptest.NewRequest(http.MethodGet, "/api/leaderboard", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/leaderboard, got %d", rr.Code)
	}
	var lbResp struct {
		Rows []LeaderboardRow `json:"rows"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&lbResp); err != nil {
		t.Fatalf("failed to decode leaderboard response: %v", err)
	}

	// 8. Test GET /api/people
	req = httptest.NewRequest(http.MethodGet, "/api/people", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/people, got %d", rr.Code)
	}
	var peopleResp struct {
		People []PersonPoolEntry `json:"people"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&peopleResp); err != nil {
		t.Fatalf("failed to decode people response: %v", err)
	}
	if len(peopleResp.People) == 0 {
		t.Fatalf("expected people in pool, got 0")
	}
}

func TestScheduledTasksWebAPI(t *testing.T) {
	_, router := setupTestWeb(t)

	// 1. Test GET /schedules (HTML template page)
	req := httptest.NewRequest(http.MethodGet, "/schedules", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /schedules page, got %d", rr.Code)
	}

	// 2. Test GET /api/schedules (initially empty)
	req = httptest.NewRequest(http.MethodGet, "/api/schedules", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/schedules, got %d", rr.Code)
	}
	var initResp struct {
		Schedules []ScheduledTaskView `json:"schedules"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&initResp); err != nil {
		t.Fatalf("failed to decode schedules: %v", err)
	}
	if len(initResp.Schedules) != 0 {
		t.Fatalf("expected 0 initial schedules, got %d", len(initResp.Schedules))
	}

	// 3. Test POST /api/schedules with invalid cron (should fail)
	badBody, _ := json.Marshal(ScheduledTaskIn{
		Name:     "Bad cron task",
		CronExpr: "not-a-cron",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewReader(badBody))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cron, got %d", rr.Code)
	}

	// 4. Test POST /api/schedules with valid task
	createBody, _ := json.Marshal(ScheduledTaskIn{
		Name:                  "Take out garbage every morning",
		Description:           "Kitchen and toilet bins",
		CronExpr:              "0 9 * * *",
		NecessaryWorkers:      1,
		EstimatedTimeMin:      10,
		AssignmentTimeoutMin:  15,
		NecessaryCapabilities: []string{"cleaning"},
		CreatorId:             "378532044558303233", // Dongalis
	})
	req = httptest.NewRequest(http.MethodPost, "/api/schedules", bytes.NewReader(createBody))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for POST /api/schedules, got %d", rr.Code)
	}
	var created ScheduledTaskView
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode created task: %v", err)
	}
	if created.ID == 0 || created.Name != "Take out garbage every morning" {
		t.Fatalf("unexpected created task: %+v", created)
	}
	if created.CreatorId != "378532044558303233" {
		t.Fatalf("expected creator Dongalis ID, got %s", created.CreatorId)
	}
	if created.NextRunAt == nil {
		t.Fatal("expected non-nil NextRunAt")
	}

	// 5. Test GET /api/schedules/{id}
	req = httptest.NewRequest(http.MethodGet, "/api/schedules/1", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/schedules/1, got %d", rr.Code)
	}

	// 6. Test PUT /api/schedules/{id}
	updateBody, _ := json.Marshal(ScheduledTaskIn{
		Name:     "Take out garbage at 8am",
		CronExpr: "0 8 * * *",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/schedules/1", bytes.NewReader(updateBody))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for PUT /api/schedules/1, got %d", rr.Code)
	}
	var updated ScheduledTaskView
	_ = json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Name != "Take out garbage at 8am" || updated.CronExpr != "0 8 * * *" {
		t.Fatalf("expected updated name and cron, got %+v", updated)
	}

	// 7. Test POST /api/schedules/{id}/toggle
	req = httptest.NewRequest(http.MethodPost, "/api/schedules/1/toggle", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for toggle, got %d", rr.Code)
	}
	var toggled ScheduledTaskView
	_ = json.NewDecoder(rr.Body).Decode(&toggled)
	if toggled.Enabled != false {
		t.Fatalf("expected enabled to be false, got %v", toggled.Enabled)
	}

	// Toggle back to enabled
	req = httptest.NewRequest(http.MethodPost, "/api/schedules/1/toggle", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for second toggle, got %d", rr.Code)
	}

	// 8. Test POST /api/schedules/{id}/run (manual trigger, verifies creator preservation)
	req = httptest.NewRequest(http.MethodPost, "/api/schedules/1/run", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for run schedule, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var runResp struct {
		Ok      bool `json:"ok"`
		ChoreId uint `json:"chore_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&runResp); err != nil {
		t.Fatalf("failed to decode run response: %v", err)
	}
	if !runResp.Ok || runResp.ChoreId == 0 {
		t.Fatalf("expected ok=true and non-zero chore_id, got %+v", runResp)
	}

	// Verify the created chore has creator set to the schedule's creator!
	req = httptest.NewRequest(http.MethodGet, "/api/chores/1", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET /api/chores/1, got %d", rr.Code)
	}
	var choreView ChoreView
	_ = json.NewDecoder(rr.Body).Decode(&choreView)
	if choreView.CreatorId != "378532044558303233" {
		t.Fatalf("expected chore CreatorId to be preserved as 378532044558303233, got %s", choreView.CreatorId)
	}

	// 9. Test DELETE /api/schedules/{id}
	req = httptest.NewRequest(http.MethodDelete, "/api/schedules/1", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d", rr.Code)
	}

	// Verify it's gone
	req = httptest.NewRequest(http.MethodGet, "/api/schedules/1", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after deletion, got %d", rr.Code)
	}
}

func TestChoreCreatorIDHandling(t *testing.T) {
	webInstance, router := setupTestWeb(t)

	// 1. Self-reported chore without CreatorId when unauthenticated should return 400 Bad Request
	body, _ := json.Marshal(ChoreCreateIn{
		Name:             "Self-reported without user",
		EstimatedTimeMin: 15,
		SelfReported:     true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/chores", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for self-reported chore without creator_id, got %d: %s", rr.Code, rr.Body.String())
	}

	// 2. Self-reported chore with CreatorId (e.g. tablet attendee picker)
	dongalisID := "378532044558303233"
	body, _ = json.Marshal(ChoreCreateIn{
		Name:             "Self-reported with chosen user",
		EstimatedTimeMin: 25,
		SelfReported:     true,
		CreatorId:        dongalisID,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/chores", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for self-reported chore with creator_id, got %d: %s", rr.Code, rr.Body.String())
	}
	var createdSelf ChoreView
	if err := json.NewDecoder(rr.Body).Decode(&createdSelf); err != nil {
		t.Fatalf("failed to decode chore: %v", err)
	}
	if createdSelf.CreatorId != dongalisID {
		t.Fatalf("expected CreatorId %s, got %s", dongalisID, createdSelf.CreatorId)
	}
	if createdSelf.CreatorName != "Dongalis (Dominik N.)" {
		t.Fatalf("expected CreatorName 'Dongalis (Dominik N.)', got '%s'", createdSelf.CreatorName)
	}

	// Check worklog was recorded for Dongalis
	worklogs, err := webInstance.storage.GetWorkLogsForChore(createdSelf.ID)
	if err != nil {
		t.Fatalf("failed to get worklogs: %v", err)
	}
	if len(worklogs) == 0 {
		t.Fatalf("expected worklog created for self-reported chore, got 0")
	}
	if worklogs[0].UserId != dongalisID || worklogs[0].TimeSpentMin != 25 {
		t.Fatalf("unexpected worklog: %+v", worklogs[0])
	}

	// 3. Authenticated session: ensure creator_id is enforced to logged-in user
	authedUser := &UserInfo{
		DiscordId: "999888777",
		Name:      "Alice In Wonderland",
		Handle:    "alice",
		IsAdmin:   false,
	}
	// Even if request payload sends spoofed creator_id, non-admin session overrides with authedUser
	body, _ = json.Marshal(ChoreCreateIn{
		Name:             "Clean fireplace",
		EstimatedTimeMin: 10,
		CreatorId:        "imposter_id",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/chores", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, authedUser))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for authed chore creation, got %d: %s", rr.Code, rr.Body.String())
	}
	var createdAuthed ChoreView
	if err := json.NewDecoder(rr.Body).Decode(&createdAuthed); err != nil {
		t.Fatalf("failed to decode chore: %v", err)
	}
	if createdAuthed.CreatorId != authedUser.DiscordId {
		t.Fatalf("expected CreatorId to be enforced to %s, got %s", authedUser.DiscordId, createdAuthed.CreatorId)
	}

	// 4. Test GET /api/me returns both flat fields and profile wrapper
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, authedUser))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/me, got %d", rr.Code)
	}
	var meResp struct {
		DiscordId string `json:"discord_id"`
		Name      string `json:"name"`
		Handle    string `json:"handle"`
		Profile   struct {
			Name          string `json:"name"`
			DiscordHandle string `json:"discord_handle"`
		} `json:"profile"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&meResp); err != nil {
		t.Fatalf("failed to decode /api/me response: %v", err)
	}
	if meResp.DiscordId != authedUser.DiscordId || meResp.Name != authedUser.Name || meResp.Profile.Name != authedUser.Name {
		t.Fatalf("unexpected /api/me response: %+v", meResp)
	}
}

func TestChoreActionsWebAPI(t *testing.T) {
	_, router := setupTestWeb(t)

	// Create chore
	choreIn := ChoreCreateIn{
		Name:             "Scrub pots",
		EstimatedTimeMin: 30,
		NecessaryWorkers: 1,
	}
	body, _ := json.Marshal(choreIn)
	req := httptest.NewRequest(http.MethodPost, "/api/chores", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("failed to create chore: %d", rr.Code)
	}
	var chore ChoreView
	json.NewDecoder(rr.Body).Decode(&chore)

	userA := &UserInfo{DiscordId: "user_a", Name: "Alice"}
	userB := &UserInfo{DiscordId: "user_b", Name: "Bob"}

	// 1. Claim chore by userA
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/chores/%d/claim", chore.ID), nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, userA))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("claim failed: %d - %s", rr.Code, rr.Body.String())
	}
	var claimResp struct {
		Ack   string    `json:"ack"`
		Chore ChoreView `json:"chore"`
	}
	json.NewDecoder(rr.Body).Decode(&claimResp)
	if claimResp.Ack == "" || len(claimResp.Chore.Claimers) != 1 || claimResp.Chore.Claimers[0].DiscordId != userA.DiscordId {
		t.Fatalf("unexpected claim response: %+v", claimResp)
	}

	// 2. Unclaim chore by userA
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/chores/%d/unclaim", chore.ID), nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, userA))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unclaim failed: %d - %s", rr.Code, rr.Body.String())
	}
	var unclaimResp struct {
		Ack   string    `json:"ack"`
		Chore ChoreView `json:"chore"`
	}
	json.NewDecoder(rr.Body).Decode(&unclaimResp)
	if len(unclaimResp.Chore.Claimers) != 0 {
		t.Fatalf("expected 0 claimers after unclaim, got %d", len(unclaimResp.Chore.Claimers))
	}

	// 3. Assign chore to userB
	assignBody, _ := json.Marshal(AssignIn{DiscordId: userB.DiscordId})
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/chores/%d/assign", chore.ID), bytes.NewReader(assignBody))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("assign failed: %d - %s", rr.Code, rr.Body.String())
	}
	var assignResp struct {
		Ack   string    `json:"ack"`
		Chore ChoreView `json:"chore"`
	}
	json.NewDecoder(rr.Body).Decode(&assignResp)
	if len(assignResp.Chore.Claimers) != 1 || assignResp.Chore.Claimers[0].DiscordId != userB.DiscordId {
		t.Fatalf("unexpected assign response: %+v", assignResp)
	}

	// 4. Report time spent
	timeBody, _ := json.Marshal(ChoreTimeIn{DiscordId: userB.DiscordId, TimeSpentMin: 45})
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/chores/%d/time", chore.ID), bytes.NewReader(timeBody))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("report time failed: %d - %s", rr.Code, rr.Body.String())
	}
	var timeResp struct {
		Ack   string    `json:"ack"`
		Chore ChoreView `json:"chore"`
	}
	json.NewDecoder(rr.Body).Decode(&timeResp)
	if timeResp.Chore.WorkedMinTotal != 45 {
		t.Fatalf("expected 45 worked min total, got %d", timeResp.Chore.WorkedMinTotal)
	}

	// 5. Unassign userB
	unassignBody, _ := json.Marshal(AssignIn{DiscordId: userB.DiscordId})
	req = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/chores/%d/unassign", chore.ID), bytes.NewReader(unassignBody))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unassign failed: %d - %s", rr.Code, rr.Body.String())
	}
}


