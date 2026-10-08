# ⛰️ Garage Trip Chores

A web app for assigning chores during the one-week Garage Trip retreat, designed for
**both mobile and a TV/big screen**. Built on FastAPI + Jinja2 + vanilla JS,
backed by SQLite, and synced in real time with the upstream
[Garage Trip Chores API](https://github.com/gdg-garage/garage-trip-chores)
(`chores.garage-trip.cz`) over REST + WebSocket.

## Pages

| URL | Screen | Purpose |
|-----|--------|---------|
| `/` | Join | First visit: name, Discord username, skills, max capacity. |
| `/feed` | Chore feed (mobile) | Live open chores; claim with a funny ack; urgent + "suggested for you" highlighting. |
| `/profile` | Profile | Edit info/skills/capacity; view your workload; log out-of-scope work (e.g. sauna). |
| `/manage` | Chore manager | Assign from the 7 presets or create a custom chore (urgent toggle, head-count scaling). |
| `/dashboard` | TV dashboard | Read-only live board + workload leaderboard, sound on new/urgent chores (mute toggle). |

## How it works

* **Upstream is the source of truth** for tasks, assignments and workload
  stats. Our backend consumes its WebSocket (`task_created`, `task_assigned`,
  `task_done`, …), keeps an in-memory cache, and re-broadcasts enriched events
  to our own clients at `/ws`.
* **SQLite** (`chores.db`) stores what upstream doesn't: local profiles
  (display name, capacity), per-chore metadata (**urgent** flag, size),
  **manual out-of-scope work**, and local claim tracking.
* **Suggestions (top 3):** required skill and remaining capacity are hard
  filters; eligible people are then ranked by **lowest current workload**
  (`normalized_total`). The top 3 are highlighted / chimed.
* **Children** (default 5) are never assignable but scale head-count chores
  (dishes, cooking, grilling) — set on the manage page per chore.

## Setup

```bash
cp .env.example .env      # then put your real CHORES_API_KEY in .env
uv sync
uv run uvicorn app.main:app --reload --host 0.0.0.0 --port 8000
```

Open `http://localhost:8000/` on phones; open `http://localhost:8000/dashboard`
on the TV.

## Notes / assumptions

* **User registration is via Discord** (the upstream bot). On Join we match your
  Discord handle against upstream `GET /users` to find your `discord_id`. If you
  haven't registered upstream yet, the profile is stored provisionally and
  reconciles automatically once you appear in `/users`.
* The upstream REST surface has no explicit "claim/ack" endpoint (that flow is
  Discord/scheduler-driven), so **claims are tracked locally** and broadcast for
  instant UI feedback; `Mark done` calls upstream `POST /tasks/{id}/done`.
* Without `CHORES_API_KEY` the app still runs, but the upstream WebSocket and
  authenticated calls are disabled (no live data).
