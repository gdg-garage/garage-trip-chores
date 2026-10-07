"""Discord OAuth login + sessions (Phase 1).

Mirrors garage-trip-digitalizace-carek: OAuth with scope `identify`, gate on the
trip's paid role, flag admins. The signed session carries the identity; we also
set the legacy `uid` cookie and upsert a local profile so the rest of the app
keeps working unchanged during the migration.
"""
from __future__ import annotations

import logging
from typing import Any, Optional

from authlib.integrations.starlette_client import OAuth
from fastapi import APIRouter, Form, HTTPException, Request
from fastapi.responses import HTMLResponse, RedirectResponse

from . import store
from .config import settings
from .discord import fetch_member_roles

log = logging.getLogger("auth")

router = APIRouter()

oauth = OAuth()
if settings.oauth_configured:
    oauth.register(
        name="discord",
        client_id=settings.DISCORD_CLIENT_ID,
        client_secret=settings.DISCORD_CLIENT_SECRET,
        access_token_url="https://discord.com/api/oauth2/token",
        authorize_url="https://discord.com/oauth2/authorize",
        api_base_url="https://discord.com/api/",
        client_kwargs={
            "scope": "identify",
            "token_endpoint_auth_method": "client_secret_post",
        },
    )

COOKIE = "uid"  # legacy identity cookie the existing endpoints still read


# --- session helpers --------------------------------------------------------

def current_user(request: Request) -> Optional[dict[str, Any]]:
    """The logged-in Discord user, or None."""
    return request.session.get("user")


def current_uid(request: Request) -> Optional[str]:
    """The logged-in user's discord_id, or None (tablet sessions have none)."""
    user = request.session.get("user")
    return user.get("discord_id") if user else None


def require_uid(request: Request) -> str:
    """FastAPI dependency: the current user's discord_id, or 401."""
    uid = current_uid(request)
    if not uid:
        raise HTTPException(status_code=401, detail="Login required")
    return uid


def is_tablet(request: Request) -> bool:
    return bool(request.session.get("tablet"))


def _discord_display(profile: dict[str, Any]) -> str:
    return profile.get("global_name") or profile.get("username") or profile.get("id")


def _avatar_url(profile: dict[str, Any]) -> Optional[str]:
    av, uid = profile.get("avatar"), profile.get("id")
    return f"https://cdn.discordapp.com/avatars/{uid}/{av}.png" if av and uid else None


def _login_user(request: Request, response, discord_id: str, name: str,
                handle: str, avatar_url: Optional[str], is_admin: bool) -> None:
    """Store identity in the session + legacy cookie, and keep a local profile
    cache of name/handle."""
    request.session["user"] = {
        "discord_id": discord_id, "name": name,
        "avatar_url": avatar_url, "is_admin": is_admin,
    }
    request.session["discord_id"] = discord_id
    response.set_cookie(COOKIE, discord_id, max_age=60 * 60 * 24 * 14, samesite="lax")

    store.upsert_profile(
        discord_id=discord_id,
        name=name,
        discord_handle=handle,
    )


# --- routes -----------------------------------------------------------------

def _error_page(title: str, body: str, status: int, debug: str = "") -> HTMLResponse:
    """A minimal dark-themed page for auth errors, so we never flash a bare
    white page against the app. Raw technical detail (if any) rides in an HTML
    comment for debugging, never in front of the user."""
    comment = f"<!-- {debug} -->" if debug else ""
    return HTMLResponse(
        "<!doctype html><meta charset='utf-8'>"
        "<meta name='viewport' content='width=device-width, initial-scale=1'>"
        f"<title>{title} — Garage Trip Chores</title>"
        "<div style=\"min-height:100vh;margin:0;display:grid;place-items:center;background:#1b1e22;"
        "color:#e8edf7;font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif\">"
        "<div style='max-width:30rem;padding:2rem 1.5rem;text-align:center'>"
        f"<h1 style='margin:.2em 0 .5em'>{title}</h1>"
        f"<p style='color:#9aa6b5;line-height:1.55'>{body}</p>"
        "<p style='margin-top:1.5rem'><a href='/' style='color:#8430ce;font-weight:700;text-decoration:none'>&larr; Back to login</a></p>"
        f"{comment}</div></div>",
        status_code=status,
    )


@router.get("/auth/discord")
async def discord_login(request: Request):
    if not settings.oauth_configured:
        return HTMLResponse(
            "<h1>Discord login isn't configured yet</h1>"
            "<p>Set DISCORD_CLIENT_ID / DISCORD_CLIENT_SECRET in the environment.</p>",
            status_code=503,
        )
    return await oauth.discord.authorize_redirect(request, settings.DISCORD_CALLBACK_URL)


@router.get("/auth/discord/callback")
async def discord_callback(request: Request):
    if not settings.oauth_configured:
        return RedirectResponse("/", status_code=302)
    try:
        token = await oauth.discord.authorize_access_token(request)
        resp = await oauth.discord.get("users/@me", token=token)
        profile = resp.json()
    except Exception as exc:  # noqa: BLE001
        # A failed token exchange is a *login* error, not a role problem — keep
        # them distinct so the message reflects the real cause.
        log.exception("discord OAuth token exchange failed")
        return _error_page(
            "Login didn't go through",
            "Discord couldn't finish signing you in. This usually clears up if you try "
            "again — and if it keeps happening, give an organiser a shout.",
            status=400,
            debug=f"{type(exc).__name__}: {exc}",
        )

    discord_id = str(profile.get("id"))
    roles = await fetch_member_roles(discord_id)
    # Gate on the paid role only when role gating is actually configured.
    if settings.role_gating_configured and settings.DISCORD_PAID_ROLE and not roles.get("has_paid"):
        log.info("login denied for %s — no paid role (found=%s)", discord_id, roles.get("found"))
        return RedirectResponse("/unauthorized", status_code=302)

    redirect = RedirectResponse("/feed", status_code=302)
    _login_user(
        request, redirect,
        discord_id=discord_id,
        name=_discord_display(profile),
        handle=profile.get("username") or discord_id,
        avatar_url=_avatar_url(profile),
        is_admin=bool(roles.get("has_admin")),
    )
    return redirect


@router.get("/auth/logout")
async def logout(request: Request):
    request.session.clear()
    resp = RedirectResponse("/", status_code=302)
    resp.delete_cookie(COOKIE)
    return resp


@router.post("/auth/tablet")
async def tablet_login(request: Request, password: str = Form(...)):
    """Shared-password login so the TV/tablet can open the dashboard."""
    if not settings.TABLET_PASSWORD or password != settings.TABLET_PASSWORD:
        return RedirectResponse("/?tablet_error=1", status_code=302)
    request.session["tablet"] = True
    return RedirectResponse("/dashboard", status_code=302)


@router.get("/unauthorized")
async def unauthorized():
    return _error_page(
        "Not on the list",
        "This Discord account isn't a paid attendee of the trip, so it can't log in. "
        "If that's not right, ping an organiser and they'll sort it out.",
        status=403,
    )
