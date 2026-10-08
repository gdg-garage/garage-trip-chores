# Deploying to Fly.io

The app is a single always-on instance with a persistent SQLite volume and one
secret (the upstream API key). WebSockets work out of the box.

## One-time setup

```bash
# 1. Install flyctl and log in
brew install flyctl          # or: curl -L https://fly.io/install.sh | sh
fly auth login

# 2. Create the app from this repo (uses the committed fly.toml; picks a name).
#    --no-deploy so we can create the volume and secret first.
fly launch --no-deploy --copy-config

# 3. Persistent disk for chores.db (same region as the app, e.g. fra)
fly volumes create chores_data --region fra --size 1

# 4. Set the upstream API key as a secret (never commit it)
fly secrets set CHORES_API_KEY=... 

# 5. Deploy
fly deploy
```

Then open the printed URL (e.g. `https://garage-trip-chores.fly.dev`).
The TV dashboard is at `/dashboard`.

## Notes

* **Keep it a single machine.** The app caches state in memory and holds one
  upstream WebSocket, so don't scale to multiple machines. The volume is
  single-attach, which naturally enforces this. `auto_stop_machines = false`
  keeps it always on so the dashboard keeps receiving live events.
* **Data persistence.** `DB_PATH=/data/chores.db` points at the mounted volume,
  so profiles/claims/templates/departures survive restarts and deploys.
* **Updates.** Push code, then `fly deploy` again.
* **Logs / status.** `fly logs`, `fly status`.
* **No app-level auth.** Anyone with the URL can use everything — share the link
  privately. Ask to add a password gate if you need it locked down.
