package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
	"github.com/gorilla/websocket"
)

func setupTestApi(t *testing.T) (*Api, *storage.Storage, *ui.Ui, func()) {
	tmpDir, err := os.MkdirTemp("", "api_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := tmpDir + "/test.sqlite"
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	storageConf := storage.Config{
		DbPath:         dbPath,
		DiscordToken:   "",
		DiscordGuildId: "test-guild",
		PresentRole:    "present",
		SkillPrefix:    "skill::",
	}

	s, err := storage.New(storageConf, logger)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create storage: %v", err)
	}

	choresLogic := chores.NewChoresLogic(s, logger, chores.Config{OversampleRatio: 0})
	uiConf := ui.Config{DiscordChannelId: "123456"}
	u := ui.NewUi(s, logger, &choresLogic, nil, uiConf)

	apiConf := Config{
		Port:    8080,
		Host:    "127.0.0.1",
		Cors:    true,
		ApiKeys: []string{},
	}

	api := NewApi(s, logger, &choresLogic, u, apiConf)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return api, s, u, cleanup
}

func TestGetTasksEmpty(t *testing.T) {
	api, _, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	var res TasksResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(res.Body) != 0 {
		t.Fatalf("Expected 0 tasks, got %d", len(res.Body))
	}
}

func TestCreateAndGetTask(t *testing.T) {
	api, _, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()

	createReq := CreateTaskInput{
		Body: TaskCreateInputBody{
			Name:                  "Wash dishes",
			EstimatedTimeMin:      20,
			NecessaryCapabilities: []string{"kitchen"},
		},
	}

	body, _ := json.Marshal(createReq.Body)
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var created TaskData
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if created.ID == 0 {
		t.Fatalf("Expected valid task ID, got 0. Body: %s", w.Body.String())
	}
	if created.Name != "Wash dishes" {
		t.Fatalf("Expected task name 'Wash dishes', got '%s'", created.Name)
	}
	if created.EstimatedTimeMin != 20 {
		t.Fatalf("Expected EstimatedTimeMin 20, got %d", created.EstimatedTimeMin)
	}

	// GET single task
	reqGet := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/tasks/%d", created.ID), nil)
	wGet := httptest.NewRecorder()
	handler.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", wGet.Code)
	}

	var fetched TaskData
	if err := json.Unmarshal(wGet.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("Failed to decode get response: %v", err)
	}

	if fetched.ID != created.ID || fetched.Name != "Wash dishes" {
		t.Fatalf("Fetched task mismatch: %+v", fetched)
	}
}

func TestTaskLifecycleViaAPI(t *testing.T) {
	api, stor, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()

	// 1. Create task
	createReq := TaskCreateInputBody{
		Name:             "Take out trash",
		EstimatedTimeMin: 15,
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var created TaskData
	json.Unmarshal(w.Body.Bytes(), &created)
	taskID := created.ID

	// 2. Ack / Claim task
	ackBody, _ := json.Marshal(TaskUserActionBody{UserId: "user-123"})
	reqAck := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tasks/%d/ack", taskID), bytes.NewReader(ackBody))
	reqAck.Header.Set("Content-Type", "application/json")
	wAck := httptest.NewRecorder()
	handler.ServeHTTP(wAck, reqAck)

	if wAck.Code != http.StatusNoContent && wAck.Code != http.StatusOK {
		t.Fatalf("Ack failed with %d: %s", wAck.Code, wAck.Body.String())
	}

	// Verify assignment created and acked
	ass, err := stor.GetChoreAssignment(taskID, "user-123")
	if err != nil || ass.Acked == nil {
		t.Fatalf("Expected acked assignment in DB: %v, %+v", err, ass)
	}

	// 3. Update / Edit task
	updateBody, _ := json.Marshal(UpdateTaskInputBody{
		Name:             "Take out trash & recycling",
		EstimatedTimeMin: 25,
	})
	reqUpdate := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/tasks/%d", taskID), bytes.NewReader(updateBody))
	reqUpdate.Header.Set("Content-Type", "application/json")
	wUpdate := httptest.NewRecorder()
	handler.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("Update failed with %d: %s", wUpdate.Code, wUpdate.Body.String())
	}

	var updated TaskData
	json.Unmarshal(wUpdate.Body.Bytes(), &updated)
	if updated.Name != "Take out trash & recycling" || updated.EstimatedTimeMin != 25 {
		t.Fatalf("Task update failed: %+v", updated)
	}

	// 4. Complete task
	reqComplete := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tasks/%d/done", taskID), nil)
	wComplete := httptest.NewRecorder()
	handler.ServeHTTP(wComplete, reqComplete)

	if wComplete.Code != http.StatusNoContent && wComplete.Code != http.StatusOK {
		t.Fatalf("Complete failed with %d: %s", wComplete.Code, wComplete.Body.String())
	}

	// Verify chore is completed in DB and worklog created
	dbChore, _ := stor.GetChore(taskID)
	if dbChore.Completed == nil {
		t.Fatalf("Expected chore to be completed in DB")
	}

	wl, err := stor.GetWorkLogForChoreAndUser(taskID, "user-123")
	if err != nil || wl.TimeSpentMin != 25 {
		t.Fatalf("Expected worklog for user-123: %v, %+v", err, wl)
	}

	// 5. Help on task
	helpBody, _ := json.Marshal(TaskUserActionBody{UserId: "user-456"})
	reqHelp := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tasks/%d/help", taskID), bytes.NewReader(helpBody))
	reqHelp.Header.Set("Content-Type", "application/json")
	wHelp := httptest.NewRecorder()
	handler.ServeHTTP(wHelp, reqHelp)

	if wHelp.Code != http.StatusNoContent && wHelp.Code != http.StatusOK {
		t.Fatalf("Help failed with %d: %s", wHelp.Code, wHelp.Body.String())
	}

	wl456, err := stor.GetWorkLogForChoreAndUser(taskID, "user-456")
	if err != nil || wl456.TimeSpentMin != 25 {
		t.Fatalf("Expected worklog for user-456: %v, %+v", err, wl456)
	}
}

func TestCancelTaskViaAPI(t *testing.T) {
	api, stor, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()

	createReq := TaskCreateInputBody{
		Name: "Mow lawn",
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var created TaskData
	json.Unmarshal(w.Body.Bytes(), &created)

	reqCancel := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/tasks/%d", created.ID), nil)
	wCancel := httptest.NewRecorder()
	handler.ServeHTTP(wCancel, reqCancel)

	if wCancel.Code != http.StatusNoContent && wCancel.Code != http.StatusOK {
		t.Fatalf("Cancel failed with %d: %s", wCancel.Code, wCancel.Body.String())
	}

	dbChore, _ := stor.GetChore(created.ID)
	if dbChore.Cancelled == nil {
		t.Fatalf("Expected chore to be cancelled in DB")
	}
}

func TestRejectTaskViaAPI(t *testing.T) {
	api, stor, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()

	createReq := TaskCreateInputBody{
		Name: "Clean garage",
	}
	body, _ := json.Marshal(createReq)
	req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var created TaskData
	json.Unmarshal(w.Body.Bytes(), &created)

	// Assign user
	stor.AssignChore(storage.Chore{ID: created.ID}, "user-reject")

	// Reject via API
	rejBody, _ := json.Marshal(TaskUserActionBody{UserId: "user-reject"})
	reqRej := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/tasks/%d/reject", created.ID), bytes.NewReader(rejBody))
	reqRej.Header.Set("Content-Type", "application/json")
	wRej := httptest.NewRecorder()
	handler.ServeHTTP(wRej, reqRej)

	if wRej.Code != http.StatusNoContent && wRej.Code != http.StatusOK {
		t.Fatalf("Reject failed with %d: %s", wRej.Code, wRej.Body.String())
	}

	ass, err := stor.GetChoreAssignment(created.ID, "user-reject")
	if err != nil || ass.Refused == nil {
		t.Fatalf("Expected refused assignment in DB: %v, %+v", err, ass)
	}
}

func TestMetadataAndStatsEndpoints(t *testing.T) {
	api, _, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()

	// GET /users
	wUsers := httptest.NewRecorder()
	handler.ServeHTTP(wUsers, httptest.NewRequest(http.MethodGet, "/users", nil))
	if wUsers.Code != http.StatusOK {
		t.Fatalf("GET /users failed: %d", wUsers.Code)
	}

	// GET /stats
	wStats := httptest.NewRecorder()
	handler.ServeHTTP(wStats, httptest.NewRequest(http.MethodGet, "/stats", nil))
	if wStats.Code != http.StatusOK {
		t.Fatalf("GET /stats failed: %d", wStats.Code)
	}

	// GET /health
	wHealth := httptest.NewRecorder()
	handler.ServeHTTP(wHealth, httptest.NewRequest(http.MethodGet, "/health", nil))
	if wHealth.Code != http.StatusOK {
		t.Fatalf("GET /health failed: %d", wHealth.Code)
	}
}

func TestWebSocketRealtimeEvents(t *testing.T) {
	api, _, _, cleanup := setupTestApi(t)
	defer cleanup()

	ts := httptest.NewServer(api.SetupRoutes())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket connection failed: %v", err)
	}
	defer wsConn.Close()

	// Perform action via API (Create task)
	createReq := TaskCreateInputBody{
		Name:             "Realtime WS Task",
		EstimatedTimeMin: 30,
	}
	body, _ := json.Marshal(createReq)
	resp, err := http.Post(ts.URL+"/tasks", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Failed to create task via HTTP: %v", err)
	}

	// Read message from WebSocket
	wsConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	var event storage.Event
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("Failed to unmarshal WS event: %v, raw: %s", err, string(msg))
	}

	if event.Type != storage.TaskCreated {
		t.Fatalf("Expected TaskCreated event, got %s", event.Type)
	}

	if event.Chore == nil || event.Chore.Name != "Realtime WS Task" {
		t.Fatalf("Expected chore 'Realtime WS Task' in event, got %+v", event.Chore)
	}
}

func TestCORSHeaders(t *testing.T) {
	api, _, _, cleanup := setupTestApi(t)
	defer cleanup()

	handler := api.SetupRoutes()

	// Preflight OPTIONS
	reqOpt := httptest.NewRequest(http.MethodOptions, "/tasks", nil)
	wOpt := httptest.NewRecorder()
	handler.ServeHTTP(wOpt, reqOpt)

	if wOpt.Code != http.StatusOK {
		t.Fatalf("Expected 200 for OPTIONS, got %d", wOpt.Code)
	}
	if wOpt.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("Expected Allow-Origin *, got %s", wOpt.Header().Get("Access-Control-Allow-Origin"))
	}
}
