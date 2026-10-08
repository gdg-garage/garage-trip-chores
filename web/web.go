package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/flosch/pongo2/v6"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"

	"github.com/gdg-garage/garage-trip-chores/chores"
	"github.com/gdg-garage/garage-trip-chores/llm"
	"github.com/gdg-garage/garage-trip-chores/storage"
	"github.com/gdg-garage/garage-trip-chores/ui"
)

//go:embed templates/*
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

func init() {
	pongo2.RegisterFilter("tojson", func(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
		b, err := json.Marshal(in.Interface())
		if err != nil {
			return pongo2.AsValue(""), nil
		}
		return pongo2.AsSafeValue(string(b)), nil
	})
}

type Web struct {
	storage     *storage.Storage
	logger      *slog.Logger
	chores      *chores.ChoresLogic
	ui          *ui.Ui
	summarizer  *llm.Summarizer
	conf        Config
	tplSet      *pongo2.TemplateSet
	cookieStore *sessions.CookieStore
	oauthConfig *oauth2.Config
	wsHub       *wsHub
}

func New(s *storage.Storage, l *slog.Logger, c *chores.ChoresLogic, u *ui.Ui, conf Config) (*Web, error) {
	subFS, err := fs.Sub(templatesFS, "templates")
	if err != nil {
		return nil, err
	}
	tplSet := pongo2.NewSet("web", pongo2.NewFSLoader(subFS))

	hub := newWsHub()
	go hub.run()

	w := &Web{
		storage: s,
		logger:  l,
		chores:  c,
		ui:      u,
		conf:    conf,
		tplSet:  tplSet,
		wsHub:   hub,
	}

	w.initAuth()
	w.listenStorageEvents()

	return w, nil
}

func (w *Web) SetSummarizer(s *llm.Summarizer) {
	w.summarizer = s
}

func (w *Web) listenStorageEvents() {
	if w.storage == nil || w.storage.Events == nil {
		return
	}

	ch := w.storage.Events.Subscribe()
	go func() {
		for event := range ch {
			if event.Chore == nil {
				continue
			}
			chore := *event.Chore
			dir := w.BuildPersonDirectory()
			worklogs, _ := w.storage.GetWorkLogsForChore(chore.ID)
			assignments, _ := w.storage.GetChoreAssignments(chore.ID)
			delayed, _ := w.storage.GetDelayedTaskForChore(chore.ID)
			view := w.BuildChoreView(chore, worklogs, assignments, delayed, dir)
			sug := w.SuggestionsFor(chore)

			eventType := "task_updated"
			switch event.Type {
			case storage.TaskCreated:
				eventType = "task_created"
			case storage.TaskDone:
				eventType = "task_done"
			case storage.TaskAcked:
				eventType = "task_claimed"
			}

			w.BroadcastWS(map[string]any{
				"type":        eventType,
				"chore":       view,
				"suggestions": sug.Top,
			})
		}
	}()
}

func (w *Web) RegisterRoutes(r chi.Router) {
	// Static assets
	staticSub, err := fs.Sub(staticFS, "static")
	if err == nil {
		r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	}

	// WebSockets (both /ws and /api/ws)
	r.Get("/ws", w.handleWS)
	r.Get("/api/ws", w.handleWS)

	// Auth routes
	r.Get("/auth/discord", w.handleDiscordLogin)
	r.Get("/auth/discord/callback", w.handleDiscordCallback)
	r.Post("/auth/tablet", w.handleTabletLogin)
	r.Get("/auth/logout", w.handleLogout)
	r.Get("/unauthorized", w.handleUnauthorized)

	// HTML pages
	r.Get("/", w.handleIndex)
	r.Get("/feed", w.handleFeed)
	r.Get("/chores/{id}", w.handleChorePage)
	r.Get("/dashboard", w.handleDashboard)
	r.Get("/manage", w.handleManage)
	r.Get("/profile", w.handleProfile)
	r.Get("/me", w.handleProfile)
	r.Get("/user/{id}", w.handleUserPage)
	r.Get("/leaderboard", w.handleLeaderboard)
	r.Get("/templates", w.handleTemplatesPage)
	r.Get("/schedules", w.handleSchedulesPage)

	// UI API routes
	r.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Get("/me", w.handleGetMe)
		apiRouter.Get("/me/manual-work", w.handleGetManualWork)
		apiRouter.Post("/me/manual-work", w.handlePostManualWork)
		apiRouter.Get("/templates", w.handleGetTemplates)
		apiRouter.Post("/templates", w.handlePostTemplate)
		apiRouter.Put("/templates/{key}", w.handlePutTemplate)
		apiRouter.Delete("/templates/{key}", w.handleDeleteTemplate)
		apiRouter.Get("/schedules", w.handleGetSchedules)
		apiRouter.Post("/schedules", w.handlePostSchedule)
		apiRouter.Get("/schedules/{id}", w.handleGetSchedule)
		apiRouter.Put("/schedules/{id}", w.handlePutSchedule)
		apiRouter.Delete("/schedules/{id}", w.handleDeleteSchedule)
		apiRouter.Post("/schedules/{id}/toggle", w.handleToggleSchedule)
		apiRouter.Post("/schedules/{id}/run", w.handleRunSchedule)
		apiRouter.Get("/skills", w.handleGetSkills)
		apiRouter.Get("/stats", w.handleGetStats)
		apiRouter.Get("/users", w.handleGetUsers)
		apiRouter.Get("/people", w.handleGetPeople)
		apiRouter.Get("/leaderboard", w.handleGetLeaderboard)
		apiRouter.Get("/users/{id}", w.handleGetUserDetail)

		apiRouter.Get("/chores", w.handleGetChores)
		apiRouter.Post("/chores", w.handlePostChore)
		apiRouter.Get("/chores/{id}", w.handleGetChore)
		apiRouter.Get("/chores/{id}/suggestions", w.handleGetSuggestions)
		apiRouter.Post("/chores/{id}/claim", w.handleClaimChore)
		apiRouter.Post("/chores/{id}/done", w.handleDoneChore)
		apiRouter.Post("/chores/{id}/help", w.handleHelpChore)
		apiRouter.Delete("/chores/{id}", w.handleDeleteChore)

		apiRouter.Post("/summary", w.handleSummary)
	})
}
