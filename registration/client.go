package registration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
)

type Client struct {
	conf       Config
	storage    *storage.Storage
	logger     *slog.Logger
	httpClient *http.Client

	mu       sync.RWMutex
	events   []storage.TripEvent
	regs     map[string][]storage.UserRegistration // eventCode -> registrations
	lastSync time.Time
}

func NewClient(conf Config, st *storage.Storage, logger *slog.Logger) *Client {
	c := &Client{
		conf:    conf,
		storage: st,
		logger:  logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		regs: make(map[string][]storage.UserRegistration),
	}
	if st != nil {
		st.SetRegistrationProvider(c)
	}
	return c
}

type apiUser struct {
	DiscordID    string `json:"discord_id"`
	DiscordIDAlt string `json:"DiscordID"`
	Username     string `json:"username"`
	UsernameAlt  string `json:"Username"`
}

func (u *apiUser) GetDiscordID() string {
	if u.DiscordID != "" {
		return u.DiscordID
	}
	return u.DiscordIDAlt
}

func (u *apiUser) GetUsername() string {
	if u.Username != "" {
		return u.Username
	}
	return u.UsernameAlt
}

type apiRegistrationItem struct {
	User             apiUser   `json:"user"`
	Event            string    `json:"event"`
	ArrivalDate      time.Time `json:"arrival_date"`
	DepartureDate    time.Time `json:"departure_date"`
	FoodRestrictions string    `json:"food_restrictions"`
	ChildrenCount    int       `json:"children_count"`
	Cancelled        bool      `json:"cancelled"`
	Paid             bool      `json:"paid"`
	Note             string    `json:"note"`
}

type apiListRegistrationsResponse struct {
	Registrations []apiRegistrationItem `json:"registrations"`
}

// FetchRegistrations calls the external registrations API for the configured default event
func (c *Client) FetchRegistrations(ctx context.Context) ([]storage.UserRegistration, error) {
	return c.FetchRegistrationsForEvent(ctx, c.conf.Event)
}

// FetchRegistrationsForEvent calls the external registrations API for a specific event
func (c *Client) FetchRegistrationsForEvent(ctx context.Context, eventCode string) ([]storage.UserRegistration, error) {
	if c.conf.ApiUrl == "" {
		return nil, nil
	}

	apiEndpoint := strings.TrimRight(c.conf.ApiUrl, "/") + "/registrations"
	if eventCode != "" {
		apiEndpoint += "?event=" + url.QueryEscape(eventCode)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.conf.ApiKey != "" {
		req.Header.Set("X-API-KEY", c.conf.ApiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registrations API returned status %d: %s", resp.StatusCode, string(body))
	}

	var data apiListRegistrationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode registrations response: %w", err)
	}

	results := make([]storage.UserRegistration, 0, len(data.Registrations))
	for _, item := range data.Registrations {
		discordID := item.User.GetDiscordID()
		if discordID == "" {
			continue
		}

		ev := item.Event
		if ev == "" {
			ev = eventCode
		}

		results = append(results, storage.UserRegistration{
			DiscordId:        discordID,
			Username:         item.User.GetUsername(),
			Event:            ev,
			ArrivalDate:      item.ArrivalDate,
			DepartureDate:    item.DepartureDate,
			FoodRestrictions: item.FoodRestrictions,
			ChildrenCount:    item.ChildrenCount,
			Cancelled:        item.Cancelled,
			Paid:             item.Paid,
			Note:             item.Note,
		})
	}

	return results, nil
}

type apiEventItem struct {
	ID          uint       `json:"id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	StartDate   *time.Time `json:"start_date"`
	EndDate     *time.Time `json:"end_date"`
	Location    string     `json:"location"`
	Description string     `json:"description"`
	Enabled     bool       `json:"enabled"`
	Status      string     `json:"status"`
}

type apiListEventsResponse struct {
	Events []apiEventItem `json:"events"`
}

// FetchEvents calls the external events API
func (c *Client) FetchEvents(ctx context.Context) ([]storage.TripEvent, error) {
	if c.conf.ApiUrl == "" {
		return nil, nil
	}

	apiEndpoint := strings.TrimRight(c.conf.ApiUrl, "/") + "/events?all=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create events request: %w", err)
	}

	if c.conf.ApiKey != "" {
		req.Header.Set("X-API-KEY", c.conf.ApiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("events HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("events API returned status %d: %s", resp.StatusCode, string(body))
	}

	var data apiListEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode events response: %w", err)
	}

	now := time.Now()
	results := make([]storage.TripEvent, 0, len(data.Events))
	for _, item := range data.Events {
		sampleMin := 10
		if strings.Contains(item.Code, "6.9") {
			sampleMin = 2
		}
		results = append(results, storage.TripEvent{
			Code:              item.Code,
			Name:              item.Name,
			StartDate:         item.StartDate,
			EndDate:           item.EndDate,
			Location:          item.Location,
			Description:       item.Description,
			Enabled:           item.Enabled,
			Status:            item.Status,
			PresenceSampleMin: sampleMin,
			UpdatedAt:         now,
		})
	}
	return results, nil
}

// Sync fetches events and registrations from the API and caches them in memory
func (c *Client) Sync(ctx context.Context) error {
	if c.conf.ApiUrl == "" {
		return nil
	}

	// 1. Fetch trip events
	events, err := c.FetchEvents(ctx)
	if err != nil {
		c.logger.Warn("Failed to sync events from API", "error", err)
	} else if len(events) > 0 {
		c.mu.Lock()
		c.events = events
		c.mu.Unlock()
		c.logger.Info("Synchronized trip events successfully", "count", len(events))
	}

	// 2. Fetch registrations for all known events
	eventCodes := make(map[string]struct{})
	c.mu.RLock()
	for _, e := range c.events {
		eventCodes[e.Code] = struct{}{}
	}
	c.mu.RUnlock()

	if c.conf.Event != "" {
		eventCodes[c.conf.Event] = struct{}{}
	}

	newRegs := make(map[string][]storage.UserRegistration)
	for code := range eventCodes {
		regs, err := c.FetchRegistrationsForEvent(ctx, code)
		if err != nil {
			c.logger.Warn("Failed to sync registrations for event", "event", code, "error", err)
		} else {
			newRegs[code] = regs
			c.logger.Info("Synchronized registrations for event", "event", code, "count", len(regs))
		}
	}

	c.mu.Lock()
	for k, v := range newRegs {
		c.regs[k] = v
	}
	c.lastSync = time.Now()
	c.mu.Unlock()

	return nil
}

// GetTripEvents returns cached trip events
func (c *Client) GetTripEvents() ([]storage.TripEvent, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]storage.TripEvent, len(c.events))
	copy(res, c.events)
	return res, nil
}

// GetUserRegistrations returns cached registrations, optionally filtered by event
func (c *Client) GetUserRegistrations(event string) ([]storage.UserRegistration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if event != "" && event != "all" {
		list := c.regs[event]
		res := make([]storage.UserRegistration, len(list))
		copy(res, list)
		return res, nil
	}

	var all []storage.UserRegistration
	for _, list := range c.regs {
		all = append(all, list...)
	}
	return all, nil
}

// GetUserRegistration returns a single registration for a user in the specified event
func (c *Client) GetUserRegistration(discordID string, event string) (*storage.UserRegistration, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if event != "" && event != "all" {
		for _, r := range c.regs[event] {
			if r.DiscordId == discordID {
				res := r
				return &res, nil
			}
		}
		return nil, nil
	}

	for _, list := range c.regs {
		for _, r := range list {
			if r.DiscordId == discordID {
				res := r
				return &res, nil
			}
		}
	}
	return nil, nil
}

// Start launches the background periodic synchronization
func (c *Client) Start(ctx context.Context, wg *sync.WaitGroup) {
	if c.conf.ApiUrl == "" {
		c.logger.Info("Registrations API URL not configured; skipping registration background sync")
		return
	}

	wg.Add(1)
	defer wg.Done()

	intervalMin := c.conf.SyncIntervalMin
	if intervalMin <= 0 {
		intervalMin = 15
	}
	ticker := time.NewTicker(time.Duration(intervalMin) * time.Minute)
	defer ticker.Stop()

	c.logger.Info("Starting registration API synchronization worker", "url", c.conf.ApiUrl, "event", c.conf.Event, "interval_min", intervalMin)

	// Run initial sync immediately
	if err := c.Sync(ctx); err != nil {
		c.logger.Warn("Initial registration sync failed, will retry on ticker", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Registration sync worker stopped")
			return
		case <-ticker.C:
			if err := c.Sync(ctx); err != nil {
				c.logger.Warn("Periodic registration sync failed", "error", err)
			}
		}
	}
}

