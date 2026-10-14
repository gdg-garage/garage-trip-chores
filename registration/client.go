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
}

func NewClient(conf Config, storage *storage.Storage, logger *slog.Logger) *Client {
	return &Client{
		conf:    conf,
		storage: storage,
		logger:  logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
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

// FetchRegistrations calls the external registrations API
func (c *Client) FetchRegistrations(ctx context.Context) ([]storage.UserRegistration, error) {
	if c.conf.ApiUrl == "" {
		return nil, nil
	}

	apiEndpoint := strings.TrimRight(c.conf.ApiUrl, "/") + "/registrations"
	if c.conf.Event != "" {
		apiEndpoint += "?event=" + url.QueryEscape(c.conf.Event)
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

		eventCode := item.Event
		if eventCode == "" {
			eventCode = c.conf.Event
		}

		results = append(results, storage.UserRegistration{
			DiscordId:        discordID,
			Username:         item.User.GetUsername(),
			Event:            eventCode,
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

// Sync fetches registrations from the API and saves them into the local database
func (c *Client) Sync(ctx context.Context) error {
	if c.conf.ApiUrl == "" {
		return nil
	}

	regs, err := c.FetchRegistrations(ctx)
	if err != nil {
		c.logger.Warn("Failed to sync registrations from API", "error", err)
		return err
	}

	if err := c.storage.SaveUserRegistrations(regs); err != nil {
		c.logger.Error("Failed to store registrations in database", "error", err)
		return err
	}

	c.logger.Info("Synchronized registrations successfully", "count", len(regs))
	return nil
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
