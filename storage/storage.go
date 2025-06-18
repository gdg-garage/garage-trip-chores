package storage

import (
	"log/slog"

	"github.com/bwmarrin/discordgo"
	slogGorm "github.com/orandin/slog-gorm"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Storage struct {
	db      *gorm.DB
	logger  *slog.Logger
	discord *discordgo.Session
	conf    Config
}

func dbConnect(conf Config, logger *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(conf.DbPath), &gorm.Config{
		Logger: slogGorm.New(slogGorm.WithHandler(logger.Handler())),
	})

	if err != nil {
		logger.Error("failed to connect the database", "path", conf.DbPath, "error", err)
		return nil, err
	}

	// Migrate the schema
	db.AutoMigrate(&Chore{}, &WorkLog{}, &ChoreAssignment{}, &PresenceLog{})
	return db, nil
}

func New(conf Config, logger *slog.Logger) (*Storage, error) {
	db, err := dbConnect(conf, logger)
	if err != nil {
		return nil, err
	}

	dg, err := discordConnect(conf.DiscordToken)
	if err != nil {
		return nil, err
	}

	return &Storage{
		db:      db,
		logger:  logger,
		discord: dg,
		conf:    conf,
	}, nil
}

func (s *Storage) GetDiscord() *discordgo.Session {
	if s.discord == nil {
		s.logger.Error("Discord session is not initialized")
		return nil
	}
	return s.discord
}

func (s *Storage) GetDiscordGuidId() string {
	return s.conf.DiscordGuildId
}
