package web

import (
	"bytes"
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

func TestUserApiKeyAuth(t *testing.T) {
	tmpDb, err := os.CreateTemp("", "web_apikey_test_*.db")
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
		AuthRequired: true,
		ApiKeys:      []string{"central-secret-key"},
	})
	if err != nil {
		t.Fatalf("failed to init web: %v", err)
	}

	router := chi.NewRouter()
	router.Use(webInstance.AuthMiddleware)
	webInstance.RegisterRoutes(router)

	// 1. Unauthenticated request to /api/chores should be 401
	req := httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without auth, got %d", rr.Code)
	}

	// 2. Central API key via Bearer should succeed
	req = httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	req.Header.Set("Authorization", "Bearer central-secret-key")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with central key, got %d", rr.Code)
	}

	// 3. Central API key via X-API-KEY should succeed
	req = httptest.NewRequest(http.MethodGet, "/api/chores", nil)
	req.Header.Set("X-API-KEY", "central-secret-key")
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with X-API-KEY, got %d", rr.Code)
	}

	// 4. Create a user-specific API key in storage
	_ = st.SaveUser(storage.User{
		DiscordId: "user_777",
		Name:      "Alice Tester",
		Handle:    "alice",
	})
	keyObj, rawKey, err := st.CreateUserApiKey("user_777", "My Test Script")
	if err != nil {
		t.Fatalf("failed to create user api key: %v", err)
	}

	// 5. Access /api/me using user's API key via Authorization: Bearer
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for user API key via Bearer, got %d", rr.Code)
	}
	var meResp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &meResp)
	if meResp["discord_id"] != "user_777" {
		t.Fatalf("expected discord_id user_777, got %v", meResp["discord_id"])
	}

	// 6. Access /api/me using user's API key via X-API-KEY
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("X-API-KEY", rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for user API key via X-API-KEY, got %d", rr.Code)
	}

	// 7. Test listing API keys via /api/me/api-keys
	req = httptest.NewRequest(http.MethodGet, "/api/me/api-keys", nil)
	req.Header.Set("X-API-KEY", rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 getting api keys, got %d", rr.Code)
	}
	var listResp map[string][]storage.UserApiKey
	_ = json.Unmarshal(rr.Body.Bytes(), &listResp)
	if len(listResp["api_keys"]) != 1 {
		t.Fatalf("expected 1 api key, got %d", len(listResp["api_keys"]))
	}

	// 8. Test creating another key via POST /api/me/api-keys
	bodyJSON := bytes.NewBufferString(`{"name": "Second Key"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/me/api-keys", bodyJSON)
	req.Header.Set("X-API-KEY", rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 creating api key, got %d", rr.Code)
	}
	var createdKey map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &createdKey)
	if createdKey["name"] != "Second Key" || createdKey["key"] == "" {
		t.Fatalf("unexpected created key response: %+v", createdKey)
	}

	// 9. Delete first key
	req = httptest.NewRequest(http.MethodDelete, "/api/me/api-keys/"+fmt.Sprint(keyObj.ID), nil)
	req.Header.Set("X-API-KEY", createdKey["key"].(string))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting key, got %d", rr.Code)
	}

	// 10. Verify old key can no longer authenticate
	req = httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("X-API-KEY", rawKey)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for deleted api key, got %d", rr.Code)
	}
}
