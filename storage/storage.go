package storage

import (
	"log/slog"
	"os"

	slogGorm "github.com/orandin/slog-gorm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Storage struct {
	db     *gorm.DB
	logger *slog.Logger
}

func New(conf Config, logger *slog.Logger) *Storage {
	db, err := gorm.Open(sqlite.Open(conf.DbPath), &gorm.Config{
		Logger: slogGorm.New(slogGorm.WithHandler(logger.Handler())),
	})

	if err != nil {
		logger.Error("failed to connect the database", "path", conf.DbPath, "error", err)
		os.Exit(1)
	}

	// Migrate the schema
	db.AutoMigrate(&Chore{}, &WorkLog{}, &ChoreAssignment{})

	return &Storage{
		db:     db,
		logger: logger,
	}
}
