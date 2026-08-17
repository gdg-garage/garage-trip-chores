package llm

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Embed timezone database so Europe/Prague is always available
)

type Scheduler struct {
	summarizer *Summarizer
	logger     *slog.Logger
	conf       Config
	loc        *time.Location
}

func NewScheduler(summarizer *Summarizer, logger *slog.Logger, conf Config) *Scheduler {
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		loc = time.FixedZone("CET", 1*3600)
	}
	return &Scheduler{
		summarizer: summarizer,
		logger:     logger,
		conf:       conf,
		loc:        loc,
	}
}

// NextRunDuration calculates the time remaining until the next 13:00 or 19:00 CET.
func NextRunDuration(now time.Time, loc *time.Location) time.Duration {
	nowInLoc := now.In(loc)
	y, m, d := nowInLoc.Date()

	target13 := time.Date(y, m, d, 13, 0, 0, 0, loc)
	target19 := time.Date(y, m, d, 19, 0, 0, 0, loc)

	if nowInLoc.Before(target13) {
		return target13.Sub(nowInLoc)
	}
	if nowInLoc.Before(target19) {
		return target19.Sub(nowInLoc)
	}

	// Next is 13:00 tomorrow
	nextDayTarget := time.Date(y, m, d+1, 13, 0, 0, 0, loc)
	return nextDayTarget.Sub(nowInLoc)
}

func (s *Scheduler) Run(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()

	if strings.TrimSpace(s.conf.ApiKey) == "" {
		s.logger.Info("LLM API key not configured, LLM summary scheduler disabled")
		<-ctx.Done()
		return
	}

	s.logger.Info("Starting LLM summary scheduler (configured to run at 13:00 and 19:00 CET)")

	for {
		waitDuration := NextRunDuration(time.Now(), s.loc)
		s.logger.Debug("Next LLM summary scheduled in", "duration", waitDuration.Round(time.Second))

		timer := time.NewTimer(waitDuration)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.logger.Info("LLM summary scheduler stopped")
			return
		case <-timer.C:
			s.logger.Info("Scheduled time reached; executing LLM chore summary")
			if err := s.summarizer.RunOnce(ctx); err != nil {
				s.logger.Error("Scheduled LLM chore summary failed", "error", err)
			}
		}
	}
}
