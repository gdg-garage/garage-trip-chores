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

func TestPresenceDownsamplingMigration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "downsample-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	appDbPath := filepath.Join(tmpDir, "app.sqlite")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	st, err := New(Config{DbPath: appDbPath}, logger)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// Insert 30,000 mock presence logs (2-min intervals over multiple buckets)
	baseTime := time.Date(2025, 9, 20, 12, 0, 0, 0, time.UTC)
	var logs []PresenceLog
	for i := 0; i < 30000; i++ {
		// 5 samples per 10-minute block (every 2 minutes)
		logs = append(logs, PresenceLog{
			UserId:    "dense_user",
			Timestamp: baseTime.Add(time.Duration(i*2) * time.Minute),
			Event:     "g::t::6.9",
		})
		if len(logs) == 5000 {
			if err := st.db.Create(&logs).Error; err != nil {
				t.Fatalf("failed to bulk insert logs: %v", err)
			}
			logs = nil
		}
	}
	if len(logs) > 0 {
		_ = st.db.Create(&logs)
	}

	// Insert 1 chore for g::t::6.9 so existingCount > 0 triggers the downsampling branch
	st.db.Create(&Chore{
		Name:  "Test 6.9 Chore",
		Event: "g::t::6.9",
	})

	var initialCount int64
	st.db.Model(&PresenceLog{}).Where("event = ?", "g::t::6.9").Count(&initialCount)
	if initialCount != 30000 {
		t.Fatalf("expected 30000 initial logs, got %d", initialCount)
	}

	// Run MigrateAndImportHistory (passes non-existent 2025 sqlite path, will execute the migration branch)
	if err := st.MigrateAndImportHistory(""); err != nil {
		t.Fatalf("MigrateAndImportHistory failed: %v", err)
	}

	var downsampledCount int64
	st.db.Model(&PresenceLog{}).Where("event = ?", "g::t::6.9").Count(&downsampledCount)
	// 30000 logs at 2 min = 60,000 minutes = 6,000 10-minute buckets
	if downsampledCount != 6000 {
		t.Errorf("expected 6000 downsampled logs, got %d", downsampledCount)
	}
}

