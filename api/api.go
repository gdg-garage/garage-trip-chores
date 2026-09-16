package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/llm"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

var userMentionRegex = regexp.MustCompile(`<@!?(\d+)>`)

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
	summarizer     *llm.Summarizer
	conf           Config
	hub            *WsHub
	authorizedKeys map[string]struct{}
}

func (a *Api) SetSummarizer(s *llm.Summarizer) {
	a.summarizer = s
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
		allWorkLogs, err := a.storage.GetWorkLogs()
		if err != nil {
			return nil, err
		}
		worklogsByChore := make(map[uint][]storage.WorkLog)
		for _, wl := range allWorkLogs {
			worklogsByChore[wl.ChoreId] = append(worklogsByChore[wl.ChoreId], wl)
		}
		allDelayedTasks, _ := a.storage.GetPendingDelayedTasks(time.Now().Add(365 * 24 * time.Hour))
		delayedByChore := make(map[uint]time.Time)
		for _, dt := range allDelayedTasks {
			delayedByChore[dt.ChoreID] = dt.PublishAt
		}
		var resp []TaskData
		for _, c := range choresList {
			var pubAt *time.Time
			if t, ok := delayedByChore[c.ID]; ok {
				pubAt = &t
			}
			resp = append(resp, toTaskData(c, assignmentsByChore[c.ID], worklogsByChore[c.ID], pubAt))
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
		chore, err := a.getNonDraftChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		assignments, err := a.storage.GetChoreAssignments(chore.ID)
		if err != nil {
			return nil, err
		}
		worklogs, err := a.storage.GetWorkLogsForChore(chore.ID)
		if err != nil {
			return nil, err
		}
		dt, _ := a.storage.GetDelayedTaskForChore(chore.ID)
		var pubAt *time.Time
		if dt != nil {
			pubAt = &dt.PublishAt
		}
		return &TaskCreateResponse{Body: toTaskData(chore, assignments, worklogs, pubAt)}, nil
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

		creatorId := "API"
		if input.Body.CreatorId != "" {
			creatorId = input.Body.CreatorId
		}

		name := input.Body.Name
		assigneeId := input.Body.AssigneeId
		if assigneeId == "" {
			if matches := userMentionRegex.FindStringSubmatch(name); len(matches) > 1 {
				assigneeId = matches[1]
				cleanName := strings.TrimSpace(userMentionRegex.ReplaceAllString(name, ""))
				if cleanName != "" {
					name = cleanName
				}
			}
		}

		chore := storage.Chore{
			Name:                 name,
			AssigneeId:           assigneeId,
			NecessaryWorkers:     workers,
			EstimatedTimeMin:     estTime,
			AssignmentTimeoutMin: timeoutMin,
			Deadline:             deadline,
			CreatorId:            creatorId,
			Created:              time.Now(),
			DelayMin:             input.Body.DelayMin,
			SelfReported:         input.Body.SelfReported,
		}
		if len(input.Body.NecessaryCapabilities) > 0 {
			chore.SetCapabilities(input.Body.NecessaryCapabilities)
		}

		if input.Body.DelayMin > 0 {
			saved, err := a.storage.SaveChore(chore)
			if err != nil {
				return nil, err
			}
			publishAt := time.Now().Add(time.Duration(input.Body.DelayMin) * time.Minute)
			_, err = a.storage.CreateDelayedTask(saved.ID, publishAt, input.Body.DelayMin)
			if err != nil {
				return nil, err
			}
			return &TaskCreateResponse{Body: toTaskData(saved, nil, nil, &publishAt)}, nil
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
		worklogs, _ := a.storage.GetWorkLogsForChore(saved.ID)
		return &TaskCreateResponse{Body: toTaskData(saved, assignments, worklogs)}, nil
	})

	// Edit / Update Task
	huma.Register(api, huma.Operation{
		OperationID: "update-task",
		Method:      http.MethodPut,
		Path:        "/tasks/{id}",
		Summary:     "Update task details",
	}, func(ctx context.Context, input *UpdateTaskInput) (*TaskCreateResponse, error) {
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
		updated, err := a.ui.EditChoreDetails(uint(input.ID), input.Body.Name, input.Body.NecessaryWorkers, input.Body.EstimatedTimeMin, input.Body.AssignmentTimeoutMin, input.Body.Deadline, input.Body.NecessaryCapabilities)
		if err != nil {
			return nil, err
		}
		assignments, _ := a.storage.GetChoreAssignments(updated.ID)
		worklogs, _ := a.storage.GetWorkLogsForChore(updated.ID)
		return &TaskCreateResponse{Body: toTaskData(updated, assignments, worklogs)}, nil
	})
	// Delete/Cancel task
	huma.Register(api, huma.Operation{
		OperationID: "delete-task",
		Method:      http.MethodDelete,
		Path:        "/tasks/{id}",
		Summary:     "Cancel/Delete a task",
	}, func(ctx context.Context, input *TaskActionInput) (*struct{}, error) {
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
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
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
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
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
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
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
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
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
		_, err := a.ui.HelpedChore(uint(input.ID), input.Body.UserId)
		return nil, err
	})

	// Report / Change Time Spent on Task
	huma.Register(api, huma.Operation{
		OperationID: "report-task-time",
		Method:      http.MethodPost,
		Path:        "/tasks/{id}/time",
		Summary:     "Report or update time spent on a task for a user",
	}, func(ctx context.Context, input *ReportTaskTimeInput) (*ReportTaskTimeResponse, error) {
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
		wl, err := a.ui.ReportTimeSpent(uint(input.ID), input.Body.UserId, input.Body.TimeSpentMin)
		if err != nil {
			return nil, err
		}
		return &ReportTaskTimeResponse{Body: toWorkLogData(wl)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-task-time",
		Method:      http.MethodPut,
		Path:        "/tasks/{id}/time",
		Summary:     "Update time spent on a task for a user (alias for POST /tasks/{id}/time)",
	}, func(ctx context.Context, input *ReportTaskTimeInput) (*ReportTaskTimeResponse, error) {
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
		wl, err := a.ui.ReportTimeSpent(uint(input.ID), input.Body.UserId, input.Body.TimeSpentMin)
		if err != nil {
			return nil, err
		}
		return &ReportTaskTimeResponse{Body: toWorkLogData(wl)}, nil
	})

	// Get work logs for a task
	huma.Register(api, huma.Operation{
		OperationID: "get-task-worklogs",
		Method:      http.MethodGet,
		Path:        "/tasks/{id}/worklogs",
		Summary:     "Get all work logs / reported time for a task",
	}, func(ctx context.Context, input *TaskActionInput) (*TaskWorkLogsResponse, error) {
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
		worklogs, err := a.storage.GetWorkLogsForChore(uint(input.ID))
		if err != nil {
			return nil, err
		}
		resp := make([]WorkLogData, 0, len(worklogs))
		for _, wl := range worklogs {
			resp = append(resp, toWorkLogData(wl))
		}
		return &TaskWorkLogsResponse{Body: resp}, nil
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

	// Get Skills
	huma.Register(api, huma.Operation{
		OperationID: "get-skills",
		Method:      http.MethodGet,
		Path:        "/skills",
		Summary:     "Get all available chore skills/capabilities",
	}, func(ctx context.Context, input *struct{}) (*SkillsResponse, error) {
		skills, err := a.storage.GetSkills()
		if err != nil {
			return nil, err
		}
		if skills == nil {
			skills = []string{}
		}
		return &SkillsResponse{Body: skills}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-api-skills",
		Method:      http.MethodGet,
		Path:        "/api/skills",
		Summary:     "Get all available chore skills/capabilities (alias)",
	}, func(ctx context.Context, input *struct{}) (*SkillsResponse, error) {
		skills, err := a.storage.GetSkills()
		if err != nil {
			return nil, err
		}
		if skills == nil {
			skills = []string{}
		}
		return &SkillsResponse{Body: skills}, nil
	})

	// Get Task Stats
	huma.Register(api, huma.Operation{
		OperationID: "get-task-stats",
		Method:      http.MethodGet,
		Path:        "/tasks/{id}/stats",
		Summary:     "Get stats for a specific task",
	}, func(ctx context.Context, input *TaskActionInput) (*TaskStatsResponse, error) {
		if _, err := a.getNonDraftChore(uint(input.ID)); err != nil {
			return nil, err
		}
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

	type TriggerSummaryResponse struct {
		Body struct {
			Message string `json:"message"`
		}
	}

	huma.Register(api, huma.Operation{
		OperationID: "trigger-llm-summary",
		Method:      http.MethodPost,
		Path:        "/summary",
		Summary:     "Trigger LLM chore summary",
		Description: "Manually trigger the LLM chore summary generation and post to Discord",
		Tags:        []string{"LLM"},
		Security: []map[string][]string{
			{"bearerAuth": {}},
		},
	}, func(ctx context.Context, input *struct{}) (*TriggerSummaryResponse, error) {
		if a.summarizer == nil {
			return nil, huma.Error500InternalServerError("LLM summarizer not configured")
		}
		if err := a.summarizer.RunOnce(ctx); err != nil {
			return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to run LLM summary: %v", err))
		}
		return &TriggerSummaryResponse{
			Body: struct {
				Message string `json:"message"`
			}{
				Message: "LLM summary triggered and published successfully",
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
	AssigneeId            string     `json:"assignee_id,omitempty"`
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
	// Acked lists users credited with the task: acked assignments plus anyone
	// who logged work on it (e.g. via "I helped" after completion).
	Acked     []string `json:"acked"`
	Declined  []string `json:"declined"`
	Timeouted []string `json:"timeouted"`
	// WorkLogs holds every reported time entry for the task, including "I helped" entries.
	WorkLogs []WorkLogData `json:"worklogs"`
	// WorkedMinTotal is the sum of all reported minutes across WorkLogs.
	WorkedMinTotal uint `json:"worked_min_total"`
	// DelayMin is the delay in minutes before sending/publishing the task.
	DelayMin uint `json:"delay_min,omitempty"`
	// PublishAt is the scheduled time when the task will be published and assigned.
	PublishAt *time.Time `json:"publish_at,omitempty"`
	// SelfReported indicates if the task was completed immediately by creator.
	SelfReported bool `json:"self_reported"`
}

type TasksResponse struct {
	Body []TaskData
}

type TaskCreateInputBody struct {
	Name                  string     `json:"name" doc:"Name of the chore"`
	AssigneeId            string     `json:"assignee_id,omitempty" doc:"Direct assignee ID (e.g. Discord user ID)"`
	NecessaryWorkers      uint       `json:"necessary_workers" default:"1"`
	EstimatedTimeMin      uint       `json:"estimated_time_min" default:"10"`
	AssignmentTimeoutMin  uint       `json:"assignment_timeout_min" default:"15"`
	Deadline              *time.Time `json:"deadline,omitempty"`
	NecessaryCapabilities []string   `json:"necessary_capabilities,omitempty"`
	DelayMin              uint       `json:"delay_min,omitempty" doc:"Delay in minutes before sending and scheduling the task"`
	CreatorId             string     `json:"creator_id,omitempty" doc:"Creator ID (e.g. Discord user ID). Defaults to 'API' if omitted"`
	SelfReported          bool       `json:"self_reported,omitempty" doc:"When true, task is marked as done immediately and assigned to creator"`
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
	ID   int `path:"id"`
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
	ID   int `path:"id"`
	Body TaskUserActionBody
}

type ReportTaskTimeBody struct {
	UserId       string `json:"user_id" doc:"User ID whose time spent is being reported/updated"`
	TimeSpentMin uint   `json:"time_spent_min" doc:"Time spent in minutes"`
}

type ReportTaskTimeInput struct {
	ID   int `path:"id"`
	Body ReportTaskTimeBody
}

type WorkLogData struct {
	ChoreId      uint   `json:"chore_id"`
	UserId       string `json:"user_id"`
	TimeSpentMin uint   `json:"time_spent_min"`
	SelfReported bool   `json:"self_reported"`
}

type ReportTaskTimeResponse struct {
	Body WorkLogData
}

type TaskWorkLogsResponse struct {
	Body []WorkLogData
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

type SkillsResponse struct {
	Body []string
}

type TaskStatsData struct {
	TotalTimeMin uint `json:"total_time_min"`
	WorkerCount  uint `json:"worker_count"`
}

type TaskStatsResponse struct {
	Body TaskStatsData
}

func (a *Api) getNonDraftChore(id uint) (storage.Chore, error) {
	chore, err := a.storage.GetChore(id)
	if err != nil {
		return chore, err
	}
	if chore.Draft {
		return chore, huma.Error404NotFound("task not found")
	}
	return chore, nil
}

func toWorkLogData(wl storage.WorkLog) WorkLogData {
	return WorkLogData{
		ChoreId:      wl.ChoreId,
		UserId:       wl.UserId,
		TimeSpentMin: wl.TimeSpentMin,
		SelfReported: wl.SelfReported,
	}
}

func toTaskData(chore storage.Chore, assignments []storage.ChoreAssignment, worklogs []storage.WorkLog, publishAt ...*time.Time) TaskData {
	var pubAt *time.Time
	if len(publishAt) > 0 {
		pubAt = publishAt[0]
	}
	assigned := make([]string, 0)
	acked := make([]string, 0)
	declined := make([]string, 0)
	timeouted := make([]string, 0)
	workLogData := make([]WorkLogData, 0, len(worklogs))
	var workedMinTotal uint

	ackedSet := make(map[string]struct{})
	for _, a := range assignments {
		if a.Acked != nil {
			acked = append(acked, a.UserId)
			ackedSet[a.UserId] = struct{}{}
		} else if a.Refused != nil {
			declined = append(declined, a.UserId)
		} else if a.Timeouted != nil {
			timeouted = append(timeouted, a.UserId)
		} else {
			assigned = append(assigned, a.UserId)
		}
	}

	for _, wl := range worklogs {
		workLogData = append(workLogData, toWorkLogData(wl))
		workedMinTotal += wl.TimeSpentMin
		if _, ok := ackedSet[wl.UserId]; !ok {
			acked = append(acked, wl.UserId)
			ackedSet[wl.UserId] = struct{}{}
		}
	}

	return TaskData{
		ID:                    chore.ID,
		Name:                  chore.Name,
		AssigneeId:            chore.AssigneeId,
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
		WorkLogs:              workLogData,
		WorkedMinTotal:        workedMinTotal,
		DelayMin:              chore.DelayMin,
		PublishAt:             pubAt,
		SelfReported:          chore.SelfReported,
	}
}
