package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

type Config struct {
	Port   int
	ApiKey string
}

type Api struct {
	storage *storage.Storage
	logger  *slog.Logger
	chores  *chores.ChoresLogic
	ui      *ui.Ui
	conf    Config
	hub     *WsHub
}

func NewApi(s *storage.Storage, logger *slog.Logger, c *chores.ChoresLogic, ui *ui.Ui, conf Config) *Api {
	api := &Api{
		storage: s,
		logger:  logger,
		chores:  c,
		ui:      ui,
		conf:    conf,
		hub:     NewWsHub(logger),
	}

	go api.hub.Run()

	go func() {
		sub := api.storage.Events.Subscribe()
		for event := range sub {
			api.hub.BroadcastEvent(event)
		}
	}()

	return api
}

// SetupRoutes configures the HTTP router and Huma API
func (a *Api) SetupRoutes() *chi.Mux {
	router := chi.NewRouter()

	// API Auth middleware
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for OpenAPI, AsyncAPI docs, and health checks
			if r.URL.Path == "/openapi.json" || r.URL.Path == "/openapi.yaml" || r.URL.Path == "/docs" || r.URL.Path == "/ws/docs" || r.URL.Path == "/ws/asyncapi.yaml" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer "+a.conf.ApiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	router.Use(authMiddleware)

	// Setup Huma
	config := huma.DefaultConfig("Garage Trip Chores API", "1.0.0")
	api := humachi.New(router, config)

	// Websocket endpoint doesn't need Huma (it's standard HTTP upgrade)
	router.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		a.ServeWs(w, r)
	})

	router.Get("/ws/asyncapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "docs/asyncapi.yaml")
	})

	router.Get("/ws/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html>
<html>
  <head>
    <title>WebSocket API Specs</title>
  </head>
  <body style="font-family: sans-serif; padding: 20px;">
	<h1>WebSocket AsyncAPI</h1>
	<p>View the raw AsyncAPI YAML spec here: <a href="/ws/asyncapi.yaml">/ws/asyncapi.yaml</a></p>
	<p>You can visually explore this specification by pasting the <a href="/ws/asyncapi.yaml">YAML content</a> into <a href="https://studio.asyncapi.com/" target="_blank">AsyncAPI Studio</a>.</p>
  </body>
</html>`))
	})

	// Health Check Endpoint
	huma.Register(api, huma.Operation{
		OperationID: "health-check",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Health check endpoint",
	}, func(ctx context.Context, input *struct{}) (*HealthResponse, error) {
		return &HealthResponse{Body: HealthData{Status: "ok"}}, nil
	})

	// Tasks Endpoint
	huma.Register(api, huma.Operation{
		OperationID: "get-tasks",
		Method:      http.MethodGet,
		Path:        "/tasks",
		Summary:     "Get all tasks",
	}, func(ctx context.Context, input *struct{}) (*TasksResponse, error) {
		choresList, err := a.storage.GetChores()
		if err != nil {
			return nil, err
		}
		var resp []TaskData
		for _, c := range choresList {
			resp = append(resp, toTaskData(c))
		}
		return &TasksResponse{Body: resp}, nil
	})

	// Create Task
	huma.Register(api, huma.Operation{
		OperationID: "create-task",
		Method:      http.MethodPost,
		Path:        "/tasks",
		Summary:     "Create a new task",
	}, func(ctx context.Context, input *CreateTaskInput) (*TaskCreateResponse, error) {
		chore := storage.Chore{
			Name:                  input.Body.Name,
			NecessaryWorkers:      input.Body.NecessaryWorkers,
			EstimatedTimeMin:      input.Body.EstimatedTimeMin,
			AssignmentTimeoutMin:  input.Body.AssignmentTimeoutMin,
			CreatorId:             "API",
			Created:               time.Now(),
		}
		if input.Body.Deadline != nil {
			chore.Deadline = input.Body.Deadline
		}
		if len(input.Body.NecessaryCapabilities) > 0 {
			chore.SetCapabilities(input.Body.NecessaryCapabilities)
		}

		saved, err := a.storage.SaveChore(chore)
		if err != nil {
			return nil, err
		}
		// Notify discord channel via ui somehow? Actually, Discord commands post to discord directly. Let's just create it via REST. If a dashboard wants it visible in discord it can trigger UI method, but wait! The user doesn't require tasks created in API to be posted to Discord, maybe yes. We will use a.ui.UpdateChoreMessage(saved) if it needs to be updated. Wait, for creation there is no MessageId yet, so UpdateChoreMessage won't work. The web UI will handle display.

		return &TaskCreateResponse{Body: toTaskData(saved)}, nil
	})

	// Stats Endpoint
	huma.Register(api, huma.Operation{
		OperationID: "get-stats",
		Method:      http.MethodGet,
		Path:        "/stats",
		Summary:     "Get user chore stats",
	}, func(ctx context.Context, input *struct{}) (*StatsResponse, error) {
		aggregatedStats, err := a.storage.GetAggregatedStats()
		if err != nil {
			return nil, err
		}

		usersStats := map[string]UserStats{}
		for k, s := range aggregatedStats {
			usersStats[k] = UserStats{
				WorkedCount:     s.WorkedCount,
				WorkedMin:       s.WorkedMin,
				AssignedMin:     s.AssignedMin,
				AssignedCount:   s.AssignedCount,
				TotalMin:        s.TotalMin,
				TotalCount:      s.TotalCount,
				PresentTicks:    s.PresentTicks,
				NormalizedTotal: s.NormalizedTotal,
			}
		}

		return &StatsResponse{Body: usersStats}, nil
	})

	// Action endpoints
	huma.Register(api, huma.Operation{
		OperationID: "schedule-task",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/schedule",
		Summary:     "Schedule a task (assign to users)",
	}, func(ctx context.Context, input *TaskActionInput) (*struct{}, error) {
		chore, err := a.storage.GetChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		users, err := a.storage.GetPresentUsers()
		if err != nil {
			return nil, err
		}
		
		_, err = a.chores.AssignChoresToUsers(users, chore)
		if err != nil {
			return nil, err
		}
		return nil, nil
	})

	// Delete task
	huma.Register(api, huma.Operation{
		OperationID: "delete-task",
		Method:      http.MethodDelete,
		Path:        "/tasks/{id}",
		Summary:     "Cancel/Delete a task",
	}, func(ctx context.Context, input *TaskActionInput) (*struct{}, error) {
		chore, err := a.storage.GetChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		chore.Cancel()
		_, err = a.storage.SaveChore(chore)
		return nil, err
	})

	// Complete task
	huma.Register(api, huma.Operation{
		OperationID: "complete-task",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/done",
		Summary:     "Mark a task as completed",
	}, func(ctx context.Context, input *TaskActionInput) (*struct{}, error) {
		chore, err := a.storage.GetChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		chore.Complete()
		_, err = a.storage.SaveChore(chore)
		return nil, err
	})



	// Get Users
	huma.Register(api, huma.Operation{
		OperationID: "get-users",
		Method:      http.MethodGet,
		Path:        "/users",
		Summary:     "Get all present users",
	}, func(ctx context.Context, input *struct{}) (*UsersResponse, error) {
		users, err := a.storage.GetPresentUsers()
		if err != nil {
			return nil, err
		}
		var resp []UserData
		for _, u := range users {
			resp = append(resp, UserData{
				DiscordId:    u.DiscordId,
				Handle:       u.Handle,
				Capabilities: u.Capabilities,
			})
		}
		return &UsersResponse{Body: resp}, nil
	})

	// Get Task Stats
	huma.Register(api, huma.Operation{
		OperationID: "get-task-stats",
		Method:      http.MethodGet,
		Path:        "/tasks/{id}/stats",
		Summary:     "Get stats for a specific task",
	}, func(ctx context.Context, input *TaskActionInput) (*TaskStatsResponse, error) {
		worklogs, err := a.storage.GetWorkLogsForChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		
		var totalTime uint
		workerIdMap := make(map[string]struct{})
		for _, log := range worklogs {
			totalTime += log.TimeSpentMin
			workerIdMap[log.UserId] = struct{}{}
		}

		return &TaskStatsResponse{
			Body: TaskStatsData{
				TotalTimeMin: totalTime,
				WorkerCount:  uint(len(workerIdMap)),
			},
		}, nil
	})

	return router
}

func (a *Api) Run(ctx context.Context) error {
	router := a.SetupRoutes()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", a.conf.Port),
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	a.logger.Info("Starting REST API", "port", a.conf.Port)
	return srv.ListenAndServe()
}

// Schemas

type HealthData struct {
	Status string `json:"status" doc:"Status of the service"`
}

type HealthResponse struct {
	Body HealthData
}

type TaskData struct {
	ID                    uint       `json:"id"`
	Name                  string     `json:"name"`
	NecessaryWorkers      uint       `json:"necessary_workers"`
	EstimatedTimeMin      uint       `json:"estimated_time_min"`
	AssignmentTimeoutMin  uint       `json:"assignment_timeout_min"`
	CreatorId             string     `json:"creator_id"`
	Created               time.Time  `json:"created"`
	Completed             *time.Time `json:"completed,omitempty"`
	Cancelled             *time.Time `json:"cancelled,omitempty"`
	Deadline              *time.Time `json:"deadline,omitempty"`
	NecessaryCapabilities []string   `json:"necessary_capabilities"`
}

type TasksResponse struct {
	Body []TaskData
}

type TaskCreateInputBody struct {
	Name                  string     `json:"name" doc:"Name of the chore"`
	NecessaryWorkers      uint       `json:"necessary_workers" default:"1"`
	EstimatedTimeMin      uint       `json:"estimated_time_min" default:"10"`
	AssignmentTimeoutMin  uint       `json:"assignment_timeout_min" default:"15"`
	Deadline              *time.Time `json:"deadline,omitempty"`
	NecessaryCapabilities []string   `json:"necessary_capabilities,omitempty"`
}

type CreateTaskInput struct {
	Body TaskCreateInputBody
}

type TaskCreateResponse struct {
	Body TaskData
}

type TaskActionInput struct {
	ID int `path:"id"`
}

type UserStats struct {
	WorkedCount     float64 `json:"worked_count"`
	WorkedMin       float64 `json:"worked_min"`
	AssignedMin     float64 `json:"assigned_min"`
	AssignedCount   float64 `json:"assigned_count"`
	TotalMin        float64 `json:"total_min"`
	TotalCount      float64 `json:"total_count"`
	PresentTicks    int     `json:"present_ticks"`
	NormalizedTotal float64 `json:"normalized_total"`
}

type StatsResponse struct {
	Body map[string]UserStats
}

type UserData struct {
	DiscordId    string   `json:"discord_id"`
	Handle       string   `json:"handle"`
	Capabilities []string `json:"capabilities"`
}

type UsersResponse struct {
	Body []UserData
}

type TaskStatsData struct {
	TotalTimeMin uint `json:"total_time_min"`
	WorkerCount  uint `json:"worker_count"`
}

type TaskStatsResponse struct {
	Body TaskStatsData
}

func toTaskData(chore storage.Chore) TaskData {
	return TaskData{
		ID:                    chore.ID,
		Name:                  chore.Name,
		NecessaryWorkers:      chore.NecessaryWorkers,
		EstimatedTimeMin:      chore.EstimatedTimeMin,
		AssignmentTimeoutMin:  chore.AssignmentTimeoutMin,
		CreatorId:             chore.CreatorId,
		Created:               chore.Created,
		Completed:             chore.Completed,
		Cancelled:             chore.Cancelled,
		Deadline:              chore.Deadline,
		NecessaryCapabilities: chore.GetCapabilities(),
	}
}
