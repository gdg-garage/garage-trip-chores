# Garage Trip Chores

[![Go](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml/badge.svg)](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml)

Garage Trip Chores is a comprehensive task and duty scheduling system integrated with Discord, a responsive Web UI, and external event APIs. It automates chore assignments, organizes duty shifts, tracks contributions with mathematical fairness, and provides real-time updates across Discord, REST, and WebSocket APIs.

## 🚀 Live API & Dashboards
The production API and Web Dashboard is hosted at: **[chores.garage-trip.cz](https://chores.garage-trip.cz)**

*   **Web Dashboard & Kiosk**: [chores.garage-trip.cz](https://chores.garage-trip.cz)
*   **REST API Documentation**: [chores.garage-trip.cz/docs](https://chores.garage-trip.cz/docs) (OpenAPI 3.1)
*   **WebSocket Documentation**: [chores.garage-trip.cz/ws/docs](https://chores.garage-trip.cz/ws/docs) (AsyncAPI)

---

## ✨ Features

### 1. Automated Chores & Workload Balancing
*   **Fair Assignment**: Calculates a **Normalized Workload Score** for every attendee (total chore & on-call minutes adjusted for hours present on site). Attendees with lower scores are prioritized for new chores.
*   **Urgency & Priority**: Supports urgent tasks with emoji indicators (🌶️🌶️🌶️) and deadline-based reminders.
*   **Headcount Scaling**: Chores can automatically scale their estimated time and worker requirements based on active attendee and child headcounts.
*   **Off-Book & Helper Logging**: One-click "I helped" buttons and manual time logging for spontaneous contributions.

### 2. Duty Schedules & Shifts Management (v4.0.0)
*   **Shift Definitions**: Reusable shift blueprints categorized by department (Kitchen, Bar, General, etc.) with configurable shift length, active daily hours, worker counts, and linked runbook/documentation URLs.
*   **ILP Optimization Solver**:
    *   **No Overlaps**: Mathematically guarantees no participant is assigned overlapping shifts.
    *   **Fatigue Caps**: Enforces maximum shifts per person per day (`max_shifts_per_person_per_day`).
    *   **Skill Matching & Buddy Pairing**: Matches required skills (`skill::`) and automatically pairs novices with experienced leads when multi-worker shifts require expertise.
    *   **Chore Minute Rewards**: Balances duty shifts against ad-hoc chores—people who worked hard on physical chores are rewarded with fewer duty shifts.
*   **Daily Noon Auto-Freeze**: Automatically generates, balances, and locks tomorrow's duty schedule every day at 12:00 noon, announcing assignments to Discord (empty schedules without shifts or assignments are quietly suppressed).
*   **Standby Ratio**: Grants participants on-call chore credit (default: 10% of shift length, customizable per definition) for being available.
*   **Swaps & Emergency Reassignment**:
    *   **1-Click Swap Proposals**: Propose and accept shift swaps via Discord interactions or the Web UI.
    *   **"Can't Attend" Auto-Replacement**: When an attendee drops a shift, the solver automatically reassigns it to the fairest available alternative candidate.

### 3. Registration System Integration & Attendance Predictions
*   **Bidirectional Registration & Event Sync**: Integrates directly with `garage-trip-api` using `X-API-KEY` to synchronize trip events and participant registrations (`arrival_date`, `departure_date`, and cancellation status).
*   **Event-Aware Scheduling**: Shift generation and daily freeze announcements only activate before and during scheduled events or when attendees are registered/present on site. When no event is happening, freeze notifications are suppressed and no shifts are assigned.
*   **Predictive Shift Scheduling**:
    *   **Late Arrivals**: Participants arriving tomorrow are recognized by their registered arrival date and scheduled for tomorrow ahead of time, even if not yet on site.
    *   **Early Departures**: Participants departing today are excluded from tomorrow's schedule even if currently on site, preventing abandoned shifts and sudden swaps.
    *   **Presence Fallback**: Participants without registration records seamlessly fall back to real-time Discord presence (`chores::present`).

### 4. Real-time Presence Tracking
*   Uses Discord roles (default: `chores::present`) to identify who is currently available on site.
*   A background presence tracker periodically samples presence and logs **Presence Ticks** (10-minute intervals) for continuous workload normalization.

### 5. Web Dashboard & Tablet Kiosk
*   **Interactive Web UI**: Responsive desktop and mobile interface (`/`, `/feed`, `/shifts`, `/chores`, `/schedules`, `/templates`, `/leaderboard`, `/profile`).
*   **Tablet Kiosk Mode**: Simple PIN/password login for shared wall-mounted tablets in common areas.
*   **Discord OAuth2 Authentication**: Role-based access control (`g::t::orgs`) for admin actions.

### 6. Scheduled Tasks & Reusable Templates
*   **Cron-Based Recurring Chores**: Define recurring tasks with cron expressions, auto-generating chores at specified times.
*   **Template Library**: Seeded and customizable templates for common recurring chores.

### 7. AI-Powered Chore Summaries
*   Integration with Google Gemini (default: `gemini-3.7-flash` on the cost-efficient `flex` service tier).
*   Scheduled summaries run twice daily (13:00 and 19:00 CET), posting entertaining progress reports and leaderboards to Discord.

---

## 🛠 How It Works

### Tracking & Statistics
The system tracks three core metrics for every participant:
1.  **Worked Time**: Total minutes spent on completed chores plus credited standby on-call minutes.
2.  **Chore Count**: Total number of chores and shifts finalized.
3.  **Presence Ticks**: How many 10-minute intervals the user was registered/present on site.

$$\text{Normalized Workload Score} = \frac{\text{Worked Minutes}}{\max(1, \text{Presence Hours})}$$

Attendees with lower normalized scores are assigned first, ensuring fair distribution regardless of whether someone arrives for 2 days or stays the entire week.

### Capabilities (Expertise)
*   Capabilities are derived from Discord roles prefixed with `skill::` (e.g., `skill::kitchen`, `skill::sound`).
*   When a chore or shift specifies required capabilities, only members possessing those roles are eligible.
*   For multi-person shifts, buddy pairing ensures at least one qualified leader is matched with other available participants.

---

## 🤖 Discord Slash Commands

| Command | Description |
| :--- | :--- |
| `/chores` | List current open chores and their statuses |
| `/chore_create` | Interactive modal to create a new chore with deadlines and skill requirements |
| `/shifts` | View upcoming duty shifts and assignments |
| `/shift_create` | Create a new shift definition (Admin only: `g::t::orgs`) |
| `/shift_freeze` | Manually freeze and announce tomorrow's shift schedule (Admin only) |
| `/shift_swap` | Propose a shift swap with another attendee |
| `/stats` | View personal chore statistics and workload score |
| `/leaderboard` | View the global event workload and contribution leaderboard |
| `/summary` | Generate an instant AI-powered progress summary |

---

## ⚙️ Configuration Reference

Configuration can be specified in `config.yaml` or via environment variables prefixed with `CHORES_`:

| Environment Variable | Default | Description |
| :--- | :--- | :--- |
| `CHORES_LOGGER_LEVEL` | `debug` | Log level (`debug`, `info`, `warn`, `error`) |
| `CHORES_DB_DBPATH` | `data/db.sqlite` | SQLite database path |
| `CHORES_DB_DISCORDTOKEN` | - | **[Required]** Discord Bot Token |
| `CHORES_DB_DISCORDGUILDID` | - | **[Required]** Discord Guild (Server) ID |
| `CHORES_DB_PRESENTROLE` | `chores::present` | Role identifying active participants |
| `CHORES_DB_SKILLPREFIX` | `skill::` | Prefix for role-based capabilities |
| `CHORES_UI_DISCORDCHANNELID` | - | **[Required]** Target Discord channel ID for chore and shift broadcasts |
| `CHORES_SHIFTS_ADMIN_ROLE` | `g::t::orgs` | Role authorized for shift administration |
| `CHORES_SHIFTS_FREEZE_HOUR` | `12` | Hour of day (0–23) when tomorrow's schedule freezes |
| `CHORES_SHIFTS_DEFAULT_STANDBY_RATIO` | `0.10` | Default on-call time credit ratio (10%) |
| `CHORES_SHIFTS_MAX_SHIFTS_PER_DAY` | `2` | Maximum shifts per person per day |
| `CHORES_SHIFTS_TIMEZONE` | `Europe/Prague` | Scheduling timezone |
| `CHORES_SHIFTS_EVENT` | - | Event identifier for shift filtering (e.g. `gt-8`) |
| `CHORES_REGISTRATION_API_URL` | - | Base URL of `garage-trip-api` (e.g. `https://api.garage-trip.cz`) |
| `CHORES_REGISTRATION_API_KEY` | - | Org API key for querying `/registrations` |
| `CHORES_REGISTRATION_EVENT` | - | Event code for registration matching |
| `CHORES_REGISTRATION_SYNC_INTERVAL_MIN` | `15` | Sync frequency in minutes |
| `CHORES_LLM_APIKEY` | - | Google Gemini API Key for chore summaries |
| `CHORES_LLM_MODEL` | `gemini-3.7-flash` | Gemini model name |
| `CHORES_LLM_SERVICETIER` | `flex` | Gemini tier (`flex`, `standard`, `priority`) |
| `CHORES_WEB_DISCORDCLIENTID` | - | Discord OAuth2 Client ID for Web UI |
| `CHORES_WEB_DISCORDCLIENTSECRET` | - | Discord OAuth2 Client Secret |
| `CHORES_WEB_TABLETPASSWORD` | `garagetrip` | Password for tablet kiosk mode |
| `CHORES_API_PORT` | `8080` | REST/WebSocket HTTP port |
| `CHORES_API_APIKEYS` | - | Comma-separated API keys for REST API access |

---

## 🏗 Tech Stack

*   **Core**: Go 1.24+ ([slog](https://pkg.go.dev/log/slog), [gorm](https://github.com/go-gorm/gorm), [discordgo](https://github.com/bwmarrin/discordgo), [viper](https://github.com/spf13/viper))
*   **Web & API**: [chi](https://github.com/go-chi/chi), [huma](https://github.com/danielgtaylor/huma) (OpenAPI 3.1 & Swagger), [pongo2](https://github.com/flosch/pongo2) (Django-style templates)
*   **Real-time**: [gorilla/websocket](https://github.com/gorilla/websocket)
*   **Persistence**: SQLite
*   **AI Integration**: Google GenAI SDK (Gemini)
*   **Version Control**: [Jujutsu (JJ)](https://github.com/jj-vcs/jj)
