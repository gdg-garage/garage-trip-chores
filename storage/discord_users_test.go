package storage

import (
	"io"
	"log/slog"
	"os"
	"testing"
)

func TestDiscordUsersTranslation(t *testing.T) {
	tmpDb, err := os.CreateTemp("", "user_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb.Close()
	defer os.Remove(tmpDb.Name())

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := New(Config{
		DbPath: tmpDb.Name(),
	}, logger)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// 1. Verify SeedUserMap populated attendees
	name := st.ResolveUserName("378532044558303233")
	if name != "Dongalis (Dominik N.)" {
		t.Fatalf("expected 'Dongalis (Dominik N.)', got '%s'", name)
	}

	name = st.ResolveUserName("706536354271723561")
	if name != "Oťas (Jan Otisk)" {
		t.Fatalf("expected 'Oťas (Jan Otisk)', got '%s'", name)
	}

	name = st.ResolveUserName("1415037315130331247")
	if name != "Rasťo (Rasťo)" {
		t.Fatalf("expected 'Rasťo (Rasťo)', got '%s'", name)
	}

	// 2. Verify unknown ID falls back to raw ID
	name = st.ResolveUserName("999999999999999999")
	if name != "999999999999999999" {
		t.Fatalf("expected fallback to raw ID, got '%s'", name)
	}

	// 3. Verify GetAllGuildUsers returns attendees
	users, err := st.GetAllGuildUsers()
	if err != nil {
		t.Fatalf("failed to get all guild users: %v", err)
	}
	if len(users) < 30 {
		t.Fatalf("expected at least 30 attendees in guild users, got %d", len(users))
	}
}
