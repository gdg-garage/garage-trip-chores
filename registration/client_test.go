package registration

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gdg-garage/garage-trip-chores/storage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestStorage(t *testing.T) *storage.Storage {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	err = db.AutoMigrate(&storage.UserRegistration{}, &storage.UserProfile{})
	if err != nil {
		t.Fatalf("failed to auto migrate: %v", err)
	}

	// Use reflection/unexported or create Storage directly for testing
	st, err := storage.New(storage.Config{DbPath: ":memory:"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	return st
}

func TestFetchRegistrations(t *testing.T) {
	arrival := time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC)
	departure := time.Date(2026, 10, 19, 14, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "test-key" {
			t.Errorf("unexpected api key: %s", r.Header.Get("X-API-KEY"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/events" {
			resp := apiListEventsResponse{
				Events: []apiEventItem{
					{
						Code:      "gt-8",
						Name:      "Garage Trip 8",
						StartDate: &arrival,
						EndDate:   &departure,
						Enabled:   true,
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		if r.URL.Path != "/registrations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if r.URL.Query().Get("event") != "gt-8" {
			t.Errorf("unexpected event: %s", r.URL.Query().Get("event"))
		}

		resp := apiListRegistrationsResponse{
			Registrations: []apiRegistrationItem{
				{
					User: apiUser{
						DiscordID: "111",
						Username:  "Alice",
					},
					Event:         "gt-8",
					ArrivalDate:   arrival,
					DepartureDate: departure,
					Cancelled:     false,
					Paid:          true,
					Note:          "Early arrival",
				},
				{
					User: apiUser{
						DiscordID: "222",
						Username:  "Bob",
					},
					Event:         "gt-8",
					ArrivalDate:   arrival,
					DepartureDate: departure,
					Cancelled:     true,
					Paid:          false,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	st := setupTestStorage(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	conf := Config{
		ApiUrl: server.URL,
		ApiKey: "test-key",
		Event:  "gt-8",
	}

	client := NewClient(conf, st, logger)
	ctx := context.Background()

	regs, err := client.FetchRegistrations(ctx)
	if err != nil {
		t.Fatalf("FetchRegistrations failed: %v", err)
	}

	if len(regs) != 2 {
		t.Fatalf("expected 2 registrations, got %d", len(regs))
	}

	if regs[0].DiscordId != "111" || regs[0].Username != "Alice" || regs[0].Cancelled != false {
		t.Errorf("unexpected first registration: %+v", regs[0])
	}
	if regs[1].DiscordId != "222" || regs[1].Cancelled != true {
		t.Errorf("unexpected second registration: %+v", regs[1])
	}

	// Test Sync to storage
	err = client.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	storedRegs, err := st.GetUserRegistrations("gt-8")
	if err != nil {
		t.Fatalf("GetUserRegistrations failed: %v", err)
	}

	if len(storedRegs) != 2 {
		t.Fatalf("expected 2 stored registrations, got %d", len(storedRegs))
	}

	regMap, err := st.GetUserRegistrationsMap("gt-8")
	if err != nil {
		t.Fatalf("GetUserRegistrationsMap failed: %v", err)
	}

	if alice, ok := regMap["111"]; !ok || alice.Username != "Alice" {
		t.Errorf("expected Alice in map, got %+v", alice)
	}
}
