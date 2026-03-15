---
description: how to deploy the application to a docker host securely
---

### Prerequisites
*   You must have `jj` and `docker-compose` installed.
*   Your host must have a configured `traefik` instance (as implied by your global rules).

### 1. Preparing the Server Structure
Create a directory on the host (e.g. at `/opt/chores/`) containing:
*   The `docker-compose.yml` file from this repo.
*   A blank `data/` directory (ensure UID:GID matches).
*   A created `.env` file based on our updated `.env.example`.

### 2. Managing Team Secrets
For highly collaborative teams, I recommend using a Password Manager (like 1Password or Bitwarden) with a Shared Vault:
1.  Add the **Production Environment Content** to a shared Secure Note.
2.  Each team member can copy-paste it to their local `.env` only if they need to run a production-mirrored test.
3.  **DO NOT** commit the `.env` file. It is already in our `.gitignore`.

### 3. Deploying via JJ
// turbo
1.  Commit the latest logic: `jj commit -m "feat(deploy): finalize api and environment configuration"`
// turbo
2.  Push to main: `jj git push --bookmark main`
3.  On the server, pull the latest image/code and restart: `docker compose pull && docker compose up -d`

### 4. API Key Security (The "Team Key" Strategy)
Since we now support **Multi-Key Authentication**, you should:
*   Issue a unique `CHORES_API_APIKEYS` entry for each team member.
*   If a developer leaves the team or their environment is compromised, simply remove their specific key from the `.env` on the server and restart. This preserves uptime for everyone else!

### 5. Routing (Traefik)
Ensure your `docker-compose.yml` includes the Traefik labels for your host `chores.garage-trip.cz` (we have updated the documentation to include this as the default production site).
