"""HTML page routes (server-rendered with Jinja2)."""
from __future__ import annotations

from pathlib import Path

from fastapi import APIRouter, Request
from fastapi.responses import RedirectResponse
from fastapi.templating import Jinja2Templates

router = APIRouter()
templates = Jinja2Templates(directory=str(Path(__file__).resolve().parent.parent / "templates"))


@router.get("/")
async def join(request: Request):
    # Already logged in → straight to the chores.
    if request.session.get("user"):
        return RedirectResponse("/feed", status_code=302)
    return templates.TemplateResponse(request, "join.html", {"active": "join"})


@router.get("/feed")
async def feed(request: Request):
    return templates.TemplateResponse(request, "feed.html", {"active": "feed"})


@router.get("/profile")
async def profile(request: Request):
    return templates.TemplateResponse(request, "profile.html", {"active": "profile"})


@router.get("/manage")
async def manage(request: Request):
    return templates.TemplateResponse(request, "manage.html", {"active": "manage"})


@router.get("/dashboard")
async def dashboard(request: Request):
    return templates.TemplateResponse(request, "dashboard.html", {"active": "dashboard"})


@router.get("/chores/{task_id}")
async def chore_detail(request: Request, task_id: int):
    return templates.TemplateResponse(
        request, "chore.html", {"active": "feed", "task_id": task_id}
    )


@router.get("/leaderboard")
async def leaderboard(request: Request):
    return templates.TemplateResponse(request, "leaderboard.html", {"active": "leaderboard"})


@router.get("/templates")
async def template_manager(request: Request):
    return templates.TemplateResponse(request, "templates.html", {"active": "manage"})


@router.get("/users/{user_id:path}")
async def user_detail(request: Request, user_id: str):
    return templates.TemplateResponse(
        request, "user.html", {"active": "leaderboard", "user_id": user_id}
    )
