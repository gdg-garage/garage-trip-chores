"""Read a Discord guild member's roles via a bot token, so login can be gated
on the trip's "paid" role and admin/present flags derived — mirroring the
`fetchMemberRoles` approach in garage-trip-digitalizace-carek.
"""
from __future__ import annotations

import logging
import time
from typing import Any, Optional

import httpx

from .config import settings

log = logging.getLogger("discord")

API = "https://discord.com/api/v10"
_ROLE_CACHE_TTL = 600  # seconds

_role_ids: dict[str, str] = {}  # role name -> role id
_role_ids_at: float = 0.0


def _headers() -> dict[str, str]:
    return {"Authorization": f"Bot {settings.DISCORD_BOT_TOKEN}"}


async def _guild_role_ids(client: httpx.AsyncClient) -> dict[str, str]:
    """Map of role name -> id for the guild (cached briefly)."""
    global _role_ids, _role_ids_at
    if _role_ids and (time.time() - _role_ids_at) < _ROLE_CACHE_TTL:
        return _role_ids
    resp = await client.get(f"{API}/guilds/{settings.DISCORD_GUILD_ID}/roles", headers=_headers())
    resp.raise_for_status()
    _role_ids = {r["name"]: r["id"] for r in resp.json()}
    _role_ids_at = time.time()
    return _role_ids


async def _guild_member(client: httpx.AsyncClient, discord_id: str) -> Optional[dict[str, Any]]:
    resp = await client.get(
        f"{API}/guilds/{settings.DISCORD_GUILD_ID}/members/{discord_id}", headers=_headers()
    )
    if resp.status_code == 404:
        return None  # not a member of the guild
    resp.raise_for_status()
    return resp.json()


async def fetch_skill_capabilities() -> list[str]:
    """Capability names from the guild's `skill::`-prefixed roles, with the
    prefix stripped (e.g. `skill::cooking` → `cooking`) to match what upstream
    `/users` reports. Returns [] when Discord isn't configured or on error."""
    if not settings.role_gating_configured:
        return []
    prefix = settings.DISCORD_SKILL_PREFIX
    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            names = await _guild_role_ids(client)  # cached ~10 min
    except Exception as exc:  # noqa: BLE001
        log.warning("fetch_skill_capabilities failed: %s", exc)
        return []
    return sorted(n[len(prefix):] for n in names if n.startswith(prefix))


async def fetch_member_roles(discord_id: str) -> dict[str, Any]:
    """Return role flags for a member.

    `found` is False when role gating isn't configured or the person isn't in
    the guild. A missing role-name config means that gate is skipped (True for
    paid so login isn't blocked; None for present = unknown).
    """
    if not settings.role_gating_configured:
        return {"found": False, "has_paid": True, "has_admin": False, "has_present": None}

    try:
        async with httpx.AsyncClient(timeout=10.0) as client:
            member = await _guild_member(client, discord_id)
            if member is None:
                return {"found": False, "has_paid": False, "has_admin": False, "has_present": False}
            names = await _guild_role_ids(client)
            member_roles = set(member.get("roles") or [])

            def has(role_name: str) -> bool:
                rid = names.get(role_name)
                return bool(rid and rid in member_roles)

            return {
                "found": True,
                "has_paid": has(settings.DISCORD_PAID_ROLE) if settings.DISCORD_PAID_ROLE else True,
                "has_admin": has(settings.DISCORD_ADMIN_ROLE) if settings.DISCORD_ADMIN_ROLE else False,
                "has_present": has(settings.DISCORD_PRESENT_ROLE) if settings.DISCORD_PRESENT_ROLE else None,
            }
    except Exception as exc:  # noqa: BLE001
        log.warning("fetch_member_roles failed for %s: %s", discord_id, exc)
        # Don't hard-fail login on a transient Discord error; treat as unknown.
        return {"found": False, "has_paid": True, "has_admin": False, "has_present": None}
