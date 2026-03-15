# Garage Trip Chores

[![Go](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml/badge.svg)](https://github.com/gdg-garage/garage-trip-chores/actions/workflows/go.yml)

Garage Trip Chores is a comprehensive task management system integrated with Discord. It automates chore assignments, tracks contributions, and provides real-time updates via Discord, REST and WebSocket APIs.

## 🚀 Live API & Dashboards
The production API is hosted at: **[chores.garage-trip.cz](https://chores.garage-trip.cz)**

*   **REST API Documentation**: [chores.garage-trip.cz/docs](https://chores.garage-trip.cz/docs) (OpenAPI)
*   **WebSocket Documentation**: [chores.garage-trip.cz/ws/docs](https://chores.garage-trip.cz/ws/docs) (AsyncAPI)

---

## ✨ Features
*   **Automated Assignment**: Mentions users for chores based on workload balancing (those who worked the least are prioritized).
*   **Presence Tracking**: Uses Discord roles (default: `chores::present`) to determine who is currently available for chores.
*   **Capabilities & Skills**: Supports role-based expertise. Some chores require specific roles prefixed with `skill::` (e.g., `skill::cooking`).
*   **Live Updates**: Event-driven architecture ensures Discord messages and external dashboards update instantly via WebSockets.
*   **Urgency & Deadlines**: Supports priority tasks (🌶️) and deadline-based scheduling.
*   **Funny Messages**: Integration with LLMs to keep chore notifications entertaining.

---

## 🛠 How It Works

### Tracking & Statistics
The system tracks three primary metrics for every user:
1.  **Worked Time**: Total minutes spent on completed chores.
2.  **Chore Count**: Total number of chores finalized.
3.  **Presence Ticks**: How many "checks" the user was marked as present.

These are combined into a **Normalized Total**, which balances workload relative to how long someone has actually been present at the location.

### Capabilities (Expertise)
Chore assignments respect user "Capabilities":
*   Users gain capabilities by having Discord roles with a specific prefix (default: `skill::`).
*   When a chore is created with "Necessary Capabilities", the assignment logic filters the candidate pool to only include users matching those skills.
*   If multiple users match, it defaults to the one with the lowest normalized workload.

### Event-Driven Sync
Every state change (Task Created, Assigned, Acked, Done) is broadcasted via an internal Event Bus.
*   **Discord UI**: Listens to the bus to live-edit messages when a task is completed via the API.
*   **WebSocket API**: Streams these events to dashboards for real-time visualization.

---

### Slash Commands
Commands are documented natively in Discord. Key commands include:
*   `/chore_create`: Create a new task with requirements.
*   `/chores`: List current open tasks.
*   `/stats`: View the global workload leaderboard.

---

### Use Cases
*   **Urgent tasks**: Use 🌶️🌶️🌶️ (1-3) in the name to flag priority.
*   **Headless Scheduling**: Use the REST API to inject recurring chores from local cron scripts or LLM managers.
*   **Manual Logging**: Create a chore, self-ACK (volunteer), and mark as done to record off-book work.

---

### Tech Stack
*   **Core**: Golang ([slog](https://pkg.go.dev/log/slog), [gorm](https://github.com/go-gorm/gorm))
*   **API**: [chi](https://github.com/go-chi/chi), [huma](https://github.com/danielgtaylor/huma) (OpenAPI 3.1)
*   **Real-time**: [gorilla/websocket](https://github.com/gorilla/websocket)
*   **Persistence**: SQLite
*   **VCS**: [JJ](https://github.com/jj-vcs/jj)

### TODO
- [ ] Proactive stats sharing with LLM integration
  - *Solved via dashboard.*
- [ ] Refuse ACK if someone worked too much compared to others
  - *Note: Better handled via communication/UI visibility than hard-coded blocks.*
