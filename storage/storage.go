package storage

import (
	"log/slog"
	"time"

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
	dg.ShouldReconnectOnError = true
	dg.ShouldRetryOnRateLimit = true
	dg.StateEnabled = true

	for {
		// Wait for the Discord session to become ready
		g, err := dg.State.Guild(conf.DiscordGuildId)
		if err == nil && g != nil && len(g.Roles) > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
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

func (s *Storage) GetDiscordGuildId() string {
	return s.conf.DiscordGuildId
}
