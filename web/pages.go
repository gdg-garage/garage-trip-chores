package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v6"
	"github.com/go-chi/chi/v5"
)

func (w *Web) renderTemplate(rw http.ResponseWriter, name string, ctx pongo2.Context) {
	tpl, err := w.tplSet.FromFile(name)
	if err != nil {
		w.logger.Error("failed to load template", "name", name, "error", err)
		http.Error(rw, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.ExecuteWriter(ctx, rw); err != nil {
		w.logger.Error("failed to render template", "name", name, "error", err)
	}
}

func (w *Web) handleIndex(rw http.ResponseWriter, r *http.Request) {
	u := w.GetCurrentUser(r)
	if u != nil {
		http.Redirect(rw, r, "/feed", http.StatusFound)
		return
	}
	if w.IsTablet(r) {
		http.Redirect(rw, r, "/dashboard", http.StatusFound)
		return
	}
	w.renderTemplate(rw, "join.html", pongo2.Context{
		"active": "",
	})
}

func (w *Web) handleFeed(rw http.ResponseWriter, r *http.Request) {
	w.renderTemplate(rw, "feed.html", pongo2.Context{
		"active": "feed",
		"user":   w.GetCurrentUser(r),
	})
}

func (w *Web) handleChorePage(rw http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, _ := strconv.Atoi(idStr)
	w.renderTemplate(rw, "chore.html", pongo2.Context{
		"active":  "feed",
		"task_id": id,
		"user":    w.GetCurrentUser(r),
	})
}

func (w *Web) handleDashboard(rw http.ResponseWriter, r *http.Request) {
	w.renderTemplate(rw, "dashboard.html", pongo2.Context{
		"active": "dashboard",
		"user":   w.GetCurrentUser(r),
	})
}

func (w *Web) handleManage(rw http.ResponseWriter, r *http.Request) {
	w.renderTemplate(rw, "manage.html", pongo2.Context{
		"active": "manage",
		"user":   w.GetCurrentUser(r),
	})
}

func (w *Web) handleProfile(rw http.ResponseWriter, r *http.Request) {
	w.renderTemplate(rw, "profile.html", pongo2.Context{
		"active": "profile",
		"user":   w.GetCurrentUser(r),
	})
}

func (w *Web) handleUserPage(rw http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	// Clean path prefix if any
	userID = strings.TrimPrefix(userID, "/")
	w.renderTemplate(rw, "user.html", pongo2.Context{
		"active":  "leaderboard",
		"user_id": userID,
		"user":    w.GetCurrentUser(r),
	})
}

func (w *Web) handleLeaderboard(rw http.ResponseWriter, r *http.Request) {
	w.renderTemplate(rw, "leaderboard.html", pongo2.Context{
		"active": "leaderboard",
		"user":   w.GetCurrentUser(r),
	})
}

func (w *Web) handleTemplatesPage(rw http.ResponseWriter, r *http.Request) {
	w.renderTemplate(rw, "templates.html", pongo2.Context{
		"active": "manage",
		"user":   w.GetCurrentUser(r),
	})
}
