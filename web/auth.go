package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
)

type contextKey string

const (
	UserContextKey   contextKey = "user"
	TabletContextKey contextKey = "tablet"
	SessionCookieName           = "gt_session"
	LegacyCookieName            = "uid"
)

func (w *Web) initAuth() {
	secret := w.conf.SessionSecret
	if secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		secret = hex.EncodeToString(b)
	}
	w.cookieStore = sessions.NewCookieStore([]byte(secret))
	w.cookieStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 14,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	w.oauthConfig = &oauth2.Config{
		ClientID:     w.conf.DiscordClientId,
		ClientSecret: w.conf.DiscordClientSecret,
		RedirectURL:  w.conf.DiscordCallbackUrl,
		Scopes:       []string{"identify"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
	}
}

func (w *Web) GetCurrentUser(r *http.Request) *UserInfo {
	val := r.Context().Value(UserContextKey)
	if val != nil {
		if u, ok := val.(*UserInfo); ok {
			return u
		}
	}
	return nil
}

func (w *Web) IsTablet(r *http.Request) bool {
	val := r.Context().Value(TabletContextKey)
	if val != nil {
		if t, ok := val.(bool); ok {
			return t
		}
	}
	return false
}

func (w *Web) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Public endpoints that never require auth
		if path == "/" ||
			strings.HasPrefix(path, "/auth/") ||
			strings.HasPrefix(path, "/static/") ||
			path == "/unauthorized" ||
			strings.HasPrefix(path, "/health") ||
			strings.HasPrefix(path, "/docs") ||
			strings.HasPrefix(path, "/openapi") ||
			strings.HasPrefix(path, "/schemas") ||
			path == "/ws" || path == "/api/ws" {
			// Try reading session anyway to populate context if logged in
			sess, _ := w.cookieStore.Get(r, SessionCookieName)
			ctx := r.Context()
			if sess != nil {
				if userJSON, ok := sess.Values["user"].(string); ok && userJSON != "" {
					var u UserInfo
					if json.Unmarshal([]byte(userJSON), &u) == nil {
						ctx = context.WithValue(ctx, UserContextKey, &u)
					}
				}
				if isTab, ok := sess.Values["tablet"].(bool); ok && isTab {
					ctx = context.WithValue(ctx, TabletContextKey, true)
				}
			}
			next.ServeHTTP(rw, r.WithContext(ctx))
			return
		}

		// API key authorization (for backend scripts or bots)
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			key := strings.TrimPrefix(authHeader, "Bearer ")
			for _, authorizedKey := range w.conf.ApiKeys {
				if key == authorizedKey {
					// Authorized via API key
					next.ServeHTTP(rw, r)
					return
				}
			}
		}

		// If auth is not required, let it pass
		if !w.conf.AuthRequired {
			next.ServeHTTP(rw, r)
			return
		}

		// Check session cookie
		sess, err := w.cookieStore.Get(r, SessionCookieName)
		if err == nil && sess != nil {
			isTablet, _ := sess.Values["tablet"].(bool)
			userJSON, _ := sess.Values["user"].(string)

			if isTablet {
				// Tablet is allowed on dashboard, audio, ws, and chore read endpoints
				ctx := context.WithValue(r.Context(), TabletContextKey, true)
				next.ServeHTTP(rw, r.WithContext(ctx))
				return
			}

			if userJSON != "" {
				var u UserInfo
				if json.Unmarshal([]byte(userJSON), &u) == nil && u.DiscordId != "" {
					ctx := context.WithValue(r.Context(), UserContextKey, &u)
					next.ServeHTTP(rw, r.WithContext(ctx))
					return
				}
			}
		}

		// Not authenticated
		if strings.HasPrefix(path, "/api/") {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusUnauthorized)
			rw.Write([]byte(`{"detail":"Login required"}`))
			return
		}

		http.Redirect(rw, r, "/", http.StatusFound)
	})
}

func (w *Web) handleDiscordLogin(rw http.ResponseWriter, r *http.Request) {
	if w.conf.DiscordClientId == "" || w.conf.DiscordClientSecret == "" {
		rw.WriteHeader(http.StatusServiceUnavailable)
		rw.Write([]byte("<h1>Discord login is not configured</h1><p>Set DISCORD_CLIENT_ID and DISCORD_CLIENT_SECRET.</p>"))
		return
	}

	state := "gt_state"
	url := w.oauthConfig.AuthCodeURL(state)
	http.Redirect(rw, r, url, http.StatusFound)
}

func (w *Web) handleDiscordCallback(rw http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(rw, r, "/", http.StatusFound)
		return
	}

	token, err := w.oauthConfig.Exchange(r.Context(), code)
	if err != nil {
		w.logger.Error("Discord OAuth exchange failed", "error", err)
		w.renderErrorPage(rw, "Login didn't go through", "Discord couldn't finish signing you in. Please try again.", http.StatusBadRequest)
		return
	}

	client := w.oauthConfig.Client(r.Context(), token)
	resp, err := client.Get("https://discord.com/api/users/@me")
	if err != nil {
		w.logger.Error("Failed to fetch Discord user info", "error", err)
		w.renderErrorPage(rw, "Login didn't go through", "Could not fetch your Discord profile.", http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()

	var discordUser struct {
		ID         string `json:"id"`
		Username   string `json:"username"`
		GlobalName string `json:"global_name"`
		Avatar     string `json:"avatar"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discordUser); err != nil {
		w.logger.Error("Failed to decode Discord user", "error", err)
		w.renderErrorPage(rw, "Login didn't go through", "Invalid response from Discord.", http.StatusBadRequest)
		return
	}

	displayName := discordUser.GlobalName
	if displayName == "" {
		displayName = discordUser.Username
	}
	if displayName == "" {
		displayName = discordUser.ID
	}

	avatarURL := ""
	if discordUser.Avatar != "" {
		avatarURL = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", discordUser.ID, discordUser.Avatar)
	}

	// Check member roles via Discord session if available
	hasPaid := true
	isAdmin := false
	dg := w.storage.GetDiscord()
	guildID := w.storage.GetDiscordGuildId()

	if dg != nil && guildID != "" {
		member, err := dg.GuildMember(guildID, discordUser.ID)
		if err == nil && member != nil {
			guildRoles, _ := dg.GuildRoles(guildID)
			roleNameMap := make(map[string]string)
			for _, gr := range guildRoles {
				roleNameMap[gr.ID] = gr.Name
			}

			userRoleNames := make(map[string]bool)
			for _, rid := range member.Roles {
				if rname, ok := roleNameMap[rid]; ok {
					userRoleNames[rname] = true
				}
			}

			if w.conf.DiscordPaidRole != "" {
				hasPaid = userRoleNames[w.conf.DiscordPaidRole]
			}
			if w.conf.DiscordAdminRole != "" {
				isAdmin = userRoleNames[w.conf.DiscordAdminRole]
			}
		}
	}

	if w.conf.DiscordPaidRole != "" && !hasPaid {
		http.Redirect(rw, r, "/unauthorized", http.StatusFound)
		return
	}

	u := UserInfo{
		DiscordId: discordUser.ID,
		Name:      displayName,
		Handle:    discordUser.Username,
		AvatarUrl: avatarURL,
		IsAdmin:   isAdmin,
	}

	sess, _ := w.cookieStore.Get(r, SessionCookieName)
	uBytes, _ := json.Marshal(u)
	sess.Values["user"] = string(uBytes)
	sess.Values["discord_id"] = discordUser.ID
	_ = sess.Save(r, rw)

	// Set legacy cookie
	http.SetCookie(rw, &http.Cookie{
		Name:     LegacyCookieName,
		Value:    discordUser.ID,
		Path:     "/",
		MaxAge:   86400 * 14,
		SameSite: http.SameSiteLaxMode,
	})

	_ = w.storage.UpsertProfile(discordUser.ID, displayName, discordUser.Username)

	http.Redirect(rw, r, "/feed", http.StatusFound)
}

func (w *Web) handleTabletLogin(rw http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(rw, r, "/?tablet_error=1", http.StatusFound)
		return
	}

	pass := r.FormValue("password")
	if w.conf.TabletPassword == "" || pass != w.conf.TabletPassword {
		http.Redirect(rw, r, "/?tablet_error=1", http.StatusFound)
		return
	}

	sess, _ := w.cookieStore.Get(r, SessionCookieName)
	sess.Values["tablet"] = true
	_ = sess.Save(r, rw)

	http.Redirect(rw, r, "/dashboard", http.StatusFound)
}

func (w *Web) handleLogout(rw http.ResponseWriter, r *http.Request) {
	sess, _ := w.cookieStore.Get(r, SessionCookieName)
	sess.Options.MaxAge = -1
	_ = sess.Save(r, rw)

	http.SetCookie(rw, &http.Cookie{
		Name:   LegacyCookieName,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	http.Redirect(rw, r, "/", http.StatusFound)
}

func (w *Web) handleUnauthorized(rw http.ResponseWriter, r *http.Request) {
	w.renderErrorPage(rw, "Not on the list", "This Discord account is not on the trip list. Please check with an organizer.", http.StatusForbidden)
}

func (w *Web) renderErrorPage(rw http.ResponseWriter, title, body string, status int) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(status)
	html := fmt.Sprintf(`<!doctype html><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s — Garage Trip Chores</title>
<div style="min-height:100vh;margin:0;display:grid;place-items:center;background:#1b1e22;color:#e8edf7;font-family:system-ui,-apple-system,sans-serif">
<div style="max-width:30rem;padding:2rem 1.5rem;text-align:center">
<h1 style="margin:.2em 0 .5em">%s</h1>
<p style="color:#9aa6b5;line-height:1.55">%s</p>
<p style="margin-top:1.5rem"><a href="/" style="color:#8430ce;font-weight:700;text-decoration:none">&larr; Back to login</a></p>
</div></div>`, title, title, body)
	rw.Write([]byte(html))
}
