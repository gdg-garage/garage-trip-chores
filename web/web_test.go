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
	webInstance, err := New(st, logger, &choresLogic, nil, Config{
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
