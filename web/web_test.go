package web

import (
	"bytes"
	"encoding/json"
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

