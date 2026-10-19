package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateAndImportHistory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "history-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	db2025Path := filepath.Join(tmpDir, "gt2025.sqlite")
	appDbPath := filepath.Join(tmpDir, "app.sqlite")

	// 1. Create a mock 2025 sqlite database with legacy schema
	db2025, err := gorm.Open(sqlite.Open(db2025Path), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create 2025 db: %v", err)
	}
	db2025.Exec(`
		CREATE TABLE chores (
			id integer PRIMARY KEY AUTOINCREMENT,
			name text,
			necessary_capabilities text,
			necessary_workers integer,
			estimated_time_min integer,
			assignment_timeout_min integer,
			creator_id text,
			message_id text,
			created datetime,
			completed datetime,
			cancelled datetime,
			deadline datetime,
			after_deadline_reminded numeric
		);
		CREATE TABLE work_logs (
			id integer PRIMARY KEY AUTOINCREMENT,
			user_id text,
			chore_id integer,
			time_spent_min integer,
			self_reported numeric
		);
		CREATE TABLE chore_assignments (
			id integer PRIMARY KEY AUTOINCREMENT,
			user_id text,
			chore_id integer,
			created datetime,
			acked datetime,
			refused datetime,
			timeouted datetime,
			volunteered numeric,
			deadline_reminded numeric,
			after_deadline_reminded numeric,
			reminded numeric
		);
		CREATE TABLE presence_logs (
			id integer PRIMARY KEY AUTOINCREMENT,
			user_id text,
			timestamp datetime
		);
	`)

	now := time.Now()
	db2025.Exec("INSERT INTO chores (id, name, created, completed) VALUES (1, 'Historic 2025 Chore', ?, ?)", now, now)
	db2025.Exec("INSERT INTO work_logs (id, user_id, chore_id, time_spent_min) VALUES (1, 'user1', 1, 45)")
	db2025.Exec("INSERT INTO presence_logs (id, user_id, timestamp) VALUES (1, 'user1', ?)", now)

	// Close db2025 connection so SQLite file can be attached
	sqlDB, _ := db2025.DB()
	_ = sqlDB.Close()

	// 2. Initialize application storage
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := New(Config{
		DbPath: appDbPath,
	}, logger)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// 3. Insert an existing chore in current DB without event tag
	c, err := st.SaveChore(Chore{
		Name: "Existing Chore",
	})
	if err != nil {
		t.Fatalf("failed to save chore: %v", err)
	}
	st.db.Model(&Chore{}).Where("id = ?", c.ID).Update("event", "")

	// 4. Run MigrateAndImportHistory with our mock 2025 database
	if err := st.MigrateAndImportHistory(db2025Path); err != nil {
		t.Fatalf("MigrateAndImportHistory failed: %v", err)
	}

	// 5. Verify existing chore backfilled to g::t::7.0.0
	var existingChore Chore
	if err := st.db.First(&existingChore, c.ID).Error; err != nil {
		t.Fatalf("failed to fetch existing chore: %v", err)
	}
	if existingChore.Event != "g::t::7.0.0" {
		t.Errorf("expected event g::t::7.0.0 for legacy chore, got %s", existingChore.Event)
	}

	// 6. Verify imported 2025 chore offset by +10000 and tagged with g::t::6.9
	var importedChore Chore
	if err := st.db.First(&importedChore, 10001).Error; err != nil {
		t.Fatalf("failed to fetch imported 2025 chore: %v", err)
	}
	if importedChore.Name != "Historic 2025 Chore" {
		t.Errorf("unexpected chore name: %s", importedChore.Name)
	}
	if importedChore.Event != "g::t::6.9" {
		t.Errorf("expected event g::t::6.9 for imported chore, got %s", importedChore.Event)
	}

	// 7. Verify worklog remapped to chore 10001
	var wl WorkLog
	if err := st.db.Where("chore_id = ?", 10001).First(&wl).Error; err != nil {
		t.Fatalf("failed to fetch remapped worklog: %v", err)
	}
	if wl.UserId != "user1" || wl.TimeSpentMin != 45 || wl.Event != "g::t::6.9" {
		t.Errorf("unexpected work log: %+v", wl)
	}

	// 8. Verify presence log imported with g::t::6.9
	var pl PresenceLog
	if err := st.db.Where("user_id = ? AND event = ?", "user1", "g::t::6.9").First(&pl).Error; err != nil {
		t.Fatalf("failed to fetch imported presence log: %v", err)
	}

	// 9. Verify idempotency
	if err := st.MigrateAndImportHistory(db2025Path); err != nil {
		t.Fatalf("second call to MigrateAndImportHistory failed: %v", err)
	}
	var count int64
	st.db.Model(&Chore{}).Where("event = ?", "g::t::6.9").Count(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 chore for g::t::6.9 after re-import, got %d", count)
	}
}
