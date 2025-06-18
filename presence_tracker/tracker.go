package presencetracker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type Tracker struct {
	storage *storage.Storage
	logger  *slog.Logger
	conf    Config
}

func NewTracker(storage *storage.Storage, logger *slog.Logger, conf Config) *Tracker {
	return &Tracker{
		storage: storage,
		logger:  logger,
		conf:    conf,
	}
}

func (t *Tracker) track() {
	u, err := t.storage.GetPresentUsers()
	if err != nil {
		t.logger.Error("Failed to get present users", "error", err)
	}
	for _, user := range u {
		_, err = t.storage.LogUserPresence(user.DiscordId)
		if err != nil {
			t.logger.Error("Failed to log user presence", "user", user.DiscordId, "error", err)
		}
	}
}

func (t *Tracker) RunTracker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	t.track()
	for {
		timer := time.NewTimer(time.Duration(t.conf.SamplePeriodMin) * time.Minute)
		select {
		case <-ctx.Done():
			t.logger.Debug("Tracker stopped: context cancelled", "reason", ctx.Err())
			return
		case <-timer.C:
			t.track()
		}
	}
}
