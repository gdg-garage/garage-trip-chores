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
	Port    int      `json:"port"`
	Host    string   `json:"host"`
	Cors    bool     `json:"cors"`
	ApiKeys []string `json:"apikeys"`
}

type Api struct {
	storage        *storage.Storage
	logger         *slog.Logger
	chores         *chores.ChoresLogic
	ui             *ui.Ui
	conf           Config
	hub            *WsHub
	authorizedKeys map[string]struct{}
}

func NewApi(s *storage.Storage, logger *slog.Logger, c *chores.ChoresLogic, ui *ui.Ui, conf Config) *Api {
	auth := make(map[string]struct{})
	for _, k := range conf.ApiKeys {
		auth[k] = struct{}{}
	}

	api := &Api{
		storage:        s,
		logger:         logger,
		chores:         c,
		ui:             ui,
		conf:           conf,
		hub:            NewWsHub(logger),
		authorizedKeys: auth,
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

	// CORS middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// API Auth middleware
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(a.authorizedKeys) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for OpenAPI, AsyncAPI docs, and health checks
			if r.URL.Path == "/openapi.json" || r.URL.Path == "/openapi.yaml" || r.URL.Path == "/docs" || r.URL.Path == "/ws/docs" || r.URL.Path == "/ws/asyncapi.yaml" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}
			authHeader := r.Header.Get("Authorization")
			token := ""
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			}
			if _, ok := a.authorizedKeys[token]; !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	router.Use(authMiddleware)

	// Setup Huma
	config := huma.DefaultConfig("Garage Trip Chores API", "1.0.0")
	if len(a.authorizedKeys) > 0 {
		config.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
			"bearerAuth": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "API Key",
				Description:  "Enter your API key provided in the configuration",
			},
		}
		config.Security = []map[string][]string{
			{"bearerAuth": {}},
		}
	}
	api := humachi.New(router, config)

	// Websocket endpoint doesn't need Huma (it's standard HTTP upgrade)
	router.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		a.ServeWs(w, r)
	})
	router.Get("/api/ws", func(w http.ResponseWriter, r *http.Request) {
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
		Security:    []map[string][]string{},
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
		allAssignments, err := a.storage.GetChoresAssignments()
		if err != nil {
			return nil, err
		}
		assignmentsByChore := make(map[uint][]storage.ChoreAssignment)
		for _, a := range allAssignments {
			assignmentsByChore[a.ChoreId] = append(assignmentsByChore[a.ChoreId], a)
		}
		var resp []TaskData
		for _, c := range choresList {
			resp = append(resp, toTaskData(c, assignmentsByChore[c.ID]))
		}
		return &TasksResponse{Body: resp}, nil
	})

	// Get single task
	huma.Register(api, huma.Operation{
		OperationID: "get-task",
		Method:      http.MethodGet,
		Path:        "/tasks/{id}",
		Summary:     "Get a single task by ID",
	}, func(ctx context.Context, input *TaskActionInput) (*TaskCreateResponse, error) {
		chore, err := a.storage.GetChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		assignments, err := a.storage.GetChoreAssignments(chore.ID)
		if err != nil {
			return nil, err
		}
		return &TaskCreateResponse{Body: toTaskData(chore, assignments)}, nil
	})

	// Create Task (with bidirectional Discord sync)
	huma.Register(api, huma.Operation{
		OperationID: "create-task",
		Method:      http.MethodPost,
		Path:        "/tasks",
		Summary:     "Create a new task and publish to Discord",
	}, func(ctx context.Context, input *CreateTaskInput) (*TaskCreateResponse, error) {
		workers := input.Body.NecessaryWorkers
		if workers == 0 {
			workers = 1
		}
		estTime := input.Body.EstimatedTimeMin
		if estTime == 0 {
			estTime = 10
		}
		timeoutMin := input.Body.AssignmentTimeoutMin
		if timeoutMin == 0 {
			timeoutMin = 15
		}
		deadline := input.Body.Deadline
		if deadline == nil {
			d := time.Now().Add(24 * time.Hour)
			deadline = &d
		}

		chore := storage.Chore{
			Name:                 input.Body.Name,
			NecessaryWorkers:     workers,
			EstimatedTimeMin:     estTime,
			AssignmentTimeoutMin: timeoutMin,
			Deadline:             deadline,
			CreatorId:            "API",
			Created:              time.Now(),
		}
		if len(input.Body.NecessaryCapabilities) > 0 {
			chore.SetCapabilities(input.Body.NecessaryCapabilities)
		}

		saved, _, err := a.ui.PublishChore(chore)
		if err != nil {
			a.logger.Warn("Failed to publish chore to Discord", "error", err)
			saved, err = a.storage.SaveChore(chore)
			if err != nil {
				return nil, err
			}
		}

		assignments, _ := a.storage.GetChoreAssignments(saved.ID)
		return &TaskCreateResponse{Body: toTaskData(saved, assignments)}, nil
	})

	// Edit / Update Task
	huma.Register(api, huma.Operation{
		OperationID: "update-task",
		Method:      http.MethodPut,
		Path:        "/tasks/{id}",
		Summary:     "Update task details",
	}, func(ctx context.Context, input *UpdateTaskInput) (*TaskCreateResponse, error) {
		updated, err := a.ui.EditChoreDetails(uint(input.ID), input.Body.Name, input.Body.NecessaryWorkers, input.Body.EstimatedTimeMin, input.Body.AssignmentTimeoutMin, input.Body.Deadline, input.Body.NecessaryCapabilities)
		if err != nil {
			return nil, err
		}
		assignments, _ := a.storage.GetChoreAssignments(updated.ID)
		return &TaskCreateResponse{Body: toTaskData(updated, assignments)}, nil
	})
	// Delete/Cancel task
	huma.Register(api, huma.Operation{
		OperationID: "delete-task",
		Method:      http.MethodDelete,
		Path:        "/tasks/{id}",
		Summary:     "Cancel/Delete a task",
	}, func(ctx context.Context, input *TaskActionInput) (*struct{}, error) {
		_, err := a.ui.CancelChore(uint(input.ID))
		return nil, err
	})

	// Complete task
	huma.Register(api, huma.Operation{
		OperationID: "complete-task",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/done",
		Summary:     "Mark a task as completed",
	}, func(ctx context.Context, input *TaskActionInput) (*struct{}, error) {
		_, err := a.ui.CompleteChore(uint(input.ID))
		return nil, err
	})

	// Ack / Claim Task
	huma.Register(api, huma.Operation{
		OperationID: "ack-task",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/ack",
		Summary:     "Acknowledge / claim a task for a user",
	}, func(ctx context.Context, input *TaskUserActionInput) (*struct{}, error) {
		_, _, err := a.ui.AckChore(uint(input.ID), input.Body.UserId)
		return nil, err
	})

	// Reject Task
	huma.Register(api, huma.Operation{
		OperationID: "reject-task",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/reject",
		Summary:     "Reject a task assignment for a user",
	}, func(ctx context.Context, input *TaskUserActionInput) (*struct{}, error) {
		_, err := a.ui.RejectChore(uint(input.ID), input.Body.UserId)
		return nil, err
	})

	// Help on Task
	huma.Register(api, huma.Operation{
		OperationID: "help-task",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/help",
		Summary:     "Log work on a completed task",
	}, func(ctx context.Context, input *TaskUserActionInput) (*struct{}, error) {
		_, err := a.ui.HelpedChore(uint(input.ID), input.Body.UserId)
		return nil, err
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
	addr := fmt.Sprintf("%s:%d", a.conf.Host, a.conf.Port)
	if a.conf.Host == "" {
		addr = fmt.Sprintf(":%d", a.conf.Port)
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()

	a.logger.Info("Starting REST API", "addr", addr)
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
	Assigned              []string   `json:"assigned"`
	Acked                 []string   `json:"acked"`
	Declined              []string   `json:"declined"`
	Timeouted             []string   `json:"timeouted"`
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

type UpdateTaskInputBody struct {
	Name                  string     `json:"name,omitempty" doc:"Updated name"`
	NecessaryWorkers      uint       `json:"necessary_workers,omitempty"`
	EstimatedTimeMin      uint       `json:"estimated_time_min,omitempty"`
	AssignmentTimeoutMin  uint       `json:"assignment_timeout_min,omitempty"`
	Deadline              *time.Time `json:"deadline,omitempty"`
	NecessaryCapabilities []string   `json:"necessary_capabilities,omitempty"`
}

type UpdateTaskInput struct {
	ID   int                 `path:"id"`
	Body UpdateTaskInputBody
}

type TaskCreateResponse struct {
	Body TaskData
}

type TaskActionInput struct {
	ID int `path:"id"`
}

type TaskUserActionBody struct {
	UserId string `json:"user_id"`
}

type TaskUserActionInput struct {
	ID   int                `path:"id"`
	Body TaskUserActionBody
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

func toTaskData(chore storage.Chore, assignments []storage.ChoreAssignment) TaskData {
	assigned := make([]string, 0)
	acked := make([]string, 0)
	declined := make([]string, 0)
	timeouted := make([]string, 0)

	for _, a := range assignments {
		if a.Acked != nil {
			acked = append(acked, a.UserId)
		} else if a.Refused != nil {
			declined = append(declined, a.UserId)
		} else if a.Timeouted != nil {
			timeouted = append(timeouted, a.UserId)
		} else {
			assigned = append(assigned, a.UserId)
		}
	}

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
		Assigned:              assigned,
		Acked:                 acked,
		Declined:              declined,
		Timeouted:             timeouted,
	}
}
