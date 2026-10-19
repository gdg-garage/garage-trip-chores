package storage

import (
	"strings"
	"time"
)

// RegistrationProvider is an interface for querying external events and registrations without DB tables
type RegistrationProvider interface {
	GetTripEvents() ([]TripEvent, error)
	GetUserRegistrations(event string) ([]UserRegistration, error)
	GetUserRegistration(discordID string, event string) (*UserRegistration, error)
}

// SaveUserRegistrations stores registrations in memory (used for tests and fallback)
func (s *Storage) SaveUserRegistrations(regs []UserRegistration) error {
	s.regMockMu.Lock()
	defer s.regMockMu.Unlock()
	if s.mockRegs == nil {
		s.mockRegs = make(map[string][]UserRegistration)
	}
	for _, r := range regs {
		s.mockRegs[r.Event] = append(s.mockRegs[r.Event], r)
	}
	return nil
}

// GetUserRegistrations returns all active registrations, optionally filtered by event
func (s *Storage) GetUserRegistrations(event string) ([]UserRegistration, error) {
	if s.regProvider != nil {
		return s.regProvider.GetUserRegistrations(event)
	}
	s.regMockMu.RLock()
	defer s.regMockMu.RUnlock()
	if event != "" {
		return s.mockRegs[event], nil
	}
	var all []UserRegistration
	for _, list := range s.mockRegs {
		all = append(all, list...)
	}
	return all, nil
}

// GetUserRegistration returns the registration for a specific discord ID for the active event
func (s *Storage) GetUserRegistration(discordID string) (*UserRegistration, error) {
	return s.GetUserRegistrationForEvent(discordID, s.GetActiveEventCode())
}

// GetUserRegistrationForEvent returns the registration for a specific discord ID and event
func (s *Storage) GetUserRegistrationForEvent(discordID string, event string) (*UserRegistration, error) {
	if s.regProvider != nil {
		return s.regProvider.GetUserRegistration(discordID, event)
	}
	s.regMockMu.RLock()
	defer s.regMockMu.RUnlock()
	if event != "" {
		for _, r := range s.mockRegs[event] {
			if r.DiscordId == discordID {
				return &r, nil
			}
		}
		return nil, nil
	}
	for _, list := range s.mockRegs {
		for _, r := range list {
			if r.DiscordId == discordID {
				return &r, nil
			}
		}
	}
	return nil, nil
}

// GetUserRegistrationsMap returns a map of discord_id -> UserRegistration
func (s *Storage) GetUserRegistrationsMap(event string) (map[string]UserRegistration, error) {
	regs, err := s.GetUserRegistrations(event)
	if err != nil {
		return nil, err
	}

	res := make(map[string]UserRegistration, len(regs))
	for _, r := range regs {
		res[r.DiscordId] = r
	}
	return res, nil
}

// SaveTripEvents stores trip events in memory
func (s *Storage) SaveTripEvents(events []TripEvent) error {
	s.regMockMu.Lock()
	defer s.regMockMu.Unlock()
	s.mockEvents = events
	return nil
}

// GetTripEvents returns all known events
func (s *Storage) GetTripEvents() ([]TripEvent, error) {
	if s.regProvider != nil {
		events, err := s.regProvider.GetTripEvents()
		if err == nil && len(events) > 0 {
			return events, nil
		}
	}
	s.regMockMu.RLock()
	defer s.regMockMu.RUnlock()
	return s.mockEvents, nil
}

// SetRegistrationProvider sets the external provider (e.g. registration.Client)
func (s *Storage) SetRegistrationProvider(p RegistrationProvider) {
	s.regProvider = p
}

// IsDateInEvent returns true if target date is within any trip event's duration (inclusive of start and end dates).
func (s *Storage) IsDateInEvent(date time.Time) (bool, *TripEvent, error) {
	events, err := s.GetTripEvents()
	if err != nil {
		return false, nil, err
	}

	y, m, d := date.Date()
	targetDay := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	for _, e := range events {
		if e.StartDate != nil && e.EndDate != nil {
			sy, sm, sd := e.StartDate.Date()
			startDay := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)

			ey, em, ed := e.EndDate.Date()
			endDay := time.Date(ey, em, ed, 23, 59, 59, 0, time.UTC)

			if !targetDay.Before(startDay) && !targetDay.After(endDay) {
				return true, &e, nil
			}
		}
	}
	return false, nil, nil
}

// GetActiveEventCode returns the currently ongoing event code based on time.Now(),
// or the nearest upcoming event, or the most recent past event, or empty string.
func (s *Storage) GetActiveEventCode() string {
	events, err := s.GetTripEvents()
	if err != nil || len(events) == 0 {
		return ""
	}

	now := time.Now()
	// 1. Check if currently inside an event
	for _, e := range events {
		if e.StartDate != nil && e.EndDate != nil {
			if !now.Before(*e.StartDate) && !now.After(*e.EndDate) {
				return e.Code
			}
		}
	}

	// 2. Look for the nearest upcoming event
	var nextEvent *TripEvent
	for i := range events {
		e := &events[i]
		if e.StartDate != nil && e.StartDate.After(now) {
			if nextEvent == nil || e.StartDate.Before(*nextEvent.StartDate) {
				nextEvent = e
			}
		}
	}
	if nextEvent != nil {
		return nextEvent.Code
	}

	// 3. Fallback: most recent past event
	var latestEvent *TripEvent
	for i := range events {
		e := &events[i]
		if e.EndDate != nil {
			if latestEvent == nil || e.EndDate.After(*latestEvent.EndDate) {
				latestEvent = e
			}
		}
	}
	if latestEvent != nil {
		return latestEvent.Code
	}

	return events[0].Code
}

// ResolveEventCode returns the requested event code if valid, or falls back to GetActiveEventCode().
func (s *Storage) ResolveEventCode(requested string) string {
	if requested == "all" {
		return "all"
	}
	if requested != "" {
		return requested
	}
	return s.GetActiveEventCode()
}

// GetEventSamplePeriodMin returns the presence tracking sample interval in minutes for an event.
func (s *Storage) GetEventSamplePeriodMin(event string) int {
	events, err := s.GetTripEvents()
	if err == nil {
		for _, e := range events {
			if e.Code == event && e.PresenceSampleMin > 0 {
				return e.PresenceSampleMin
			}
		}
	}
	if strings.Contains(event, "6.9") {
		return 2
	}
	return 10 // Default 10 min
}

