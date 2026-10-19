package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestUserApiKeys(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "apikeys-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := New(Config{DbPath: dbPath}, logger)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// 1. Create API key for user1
	keyObj, rawKey, err := st.CreateUserApiKey("user_123", "Script Key")
	if err != nil {
		t.Fatalf("failed to create api key: %v", err)
	}
	if keyObj.ID == 0 {
		t.Fatalf("expected non-zero ID, got %d", keyObj.ID)
	}
	if len(rawKey) < 20 {
		t.Fatalf("rawKey too short: %s", rawKey)
	}

	// 2. Fetch list for user_123 (masked)
	keys, err := st.GetUserApiKeys("user_123")
	if err != nil {
		t.Fatalf("failed to get api keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Key == rawKey {
		t.Fatalf("expected masked key, got raw: %s", keys[0].Key)
	}

	// 3. Resolve user by raw key
	user, err := st.GetUserByApiKey(rawKey)
	if err != nil {
		t.Fatalf("failed to get user by key: %v", err)
	}
	if user.DiscordId != "user_123" {
		t.Fatalf("expected user_123, got %s", user.DiscordId)
	}

	// 4. Delete key
	if err := st.DeleteUserApiKey("user_123", keyObj.ID); err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// 5. Verify deleted
	_, err = st.GetUserByApiKey(rawKey)
	if err == nil {
		t.Fatalf("expected error getting deleted key, got nil")
	}
}
