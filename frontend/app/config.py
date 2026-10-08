"""Runtime configuration, loaded from the environment / .env file."""
from __future__ import annotations

import os
from pathlib import Path

from dotenv import load_dotenv

# Load .env from the project root (one level above this file's package).
PROJECT_ROOT = Path(__file__).resolve().parent.parent
load_dotenv(PROJECT_ROOT / ".env")


class Settings:
    # Upstream Garage Trip Chores API.
    CHORES_API_BASE: str = os.getenv("CHORES_API_BASE", "https://chores.garage-trip.cz").rstrip("/")
    CHORES_WS_URL: str = os.getenv("CHORES_WS_URL", "wss://chores.garage-trip.cz/ws")
    CHORES_API_KEY: str = os.getenv("CHORES_API_KEY", "")

    # Local persistence. STORE_BACKEND selects where app data lives: "sqlite"
    # (app/db.py) today, "api" once the persistence service exists (Phase 3).
    STORE_BACKEND: str = os.getenv("STORE_BACKEND", "sqlite").lower()
    DB_PATH: str = os.getenv("DB_PATH", str(PROJECT_ROOT / "chores.db"))

    # Retreat parameters. Little children are not assignable but add to the
    # workload (they eat, they make mess), so they scale head-count chores.
    CHILDREN_COUNT: int = int(os.getenv("CHILDREN_COUNT", "5"))

    # --- Discord OAuth / sessions (Phase 1) ---
    DISCORD_CLIENT_ID: str = os.getenv("DISCORD_CLIENT_ID", "")
    DISCORD_CLIENT_SECRET: str = os.getenv("DISCORD_CLIENT_SECRET", "")
    DISCORD_CALLBACK_URL: str = os.getenv(
        "DISCORD_CALLBACK_URL", "http://localhost:8000/auth/discord/callback"
    )
    # Bot token + guild are used to read a member's roles for gating.
    DISCORD_BOT_TOKEN: str = os.getenv("DISCORD_BOT_TOKEN", "")
    DISCORD_GUILD_ID: str = os.getenv("DISCORD_GUILD_ID", "")
    # Role names (mapped to ids at runtime). Paid gates login; admin unlocks
    # manage actions; present marks who's still on the trip.
    DISCORD_PAID_ROLE: str = os.getenv("DISCORD_PAID_ROLE", "")
    DISCORD_ADMIN_ROLE: str = os.getenv("DISCORD_ADMIN_ROLE", "")
    DISCORD_PRESENT_ROLE: str = os.getenv("DISCORD_PRESENT_ROLE", "")
    # Prefix marking a Discord role as a skill/capability (upstream default
    # `skill::`); the bare name after it is the capability (e.g. skill::cooking
    # → cooking), matching what upstream /users reports.
    DISCORD_SKILL_PREFIX: str = os.getenv("DISCORD_SKILL_PREFIX", "skill::")

    SESSION_SECRET: str = os.getenv("SESSION_SECRET", "dev-insecure-change-me")
    # Shared password so the TV/tablet can open the dashboard without OAuth.
    TABLET_PASSWORD: str = os.getenv("TABLET_PASSWORD", "")
    # When true, pages require a Discord (or tablet) session. Off by default so
    # the app keeps working while OAuth is being configured (Phase 1).
    AUTH_REQUIRED: bool = os.getenv("AUTH_REQUIRED", "false").lower() in ("1", "true", "yes")

    @property
    def has_upstream_key(self) -> bool:
        return bool(self.CHORES_API_KEY)

    @property
    def oauth_configured(self) -> bool:
        return bool(self.DISCORD_CLIENT_ID and self.DISCORD_CLIENT_SECRET)

    @property
    def role_gating_configured(self) -> bool:
        return bool(self.DISCORD_BOT_TOKEN and self.DISCORD_GUILD_ID)


settings = Settings()
