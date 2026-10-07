"""JSON API backing the pages."""
from __future__ import annotations

import random
import re

from fastapi import APIRouter, Depends, HTTPException, Request

from .. import service, store
from ..auth import current_uid, require_uid
from ..discord import fetch_skill_capabilities
from ..catalog import (
    FUNNY_ACK_MESSAGES,
    MANUAL_MARK,
    SKILLS,
    SPICY,
    size_for,
    spiciness_of,
    template_time,
)
from ..config import settings
from ..models import (
    AssignIn,
    ChoreCreateIn,
    ManualWorkIn,
    TemplateIn,
    TimeReportIn,
)
from ..suggestions import build_person_pool
from ..upstream import upstream

router = APIRouter(prefix="/api")


# --- identity / profile -----------------------------------------------------
# Identity comes from the Discord OAuth session (see app/auth.py). Name/handle
# are set from Discord at login; only skills + capacity are editable here.

@router.get("/me")
async def me(request: Request):
    uid = current_uid(request)
    if not uid:
        return {"profile": None}
    return {"profile": store.get_profile(uid), "discord_id": uid}


@router.get("/me/manual-work")
async def list_manual(uid: str = Depends(require_uid)):
    """The user's off-book work: completed, self-acked tasks marked as manual,
    read from the upstream task cache (no local storage)."""
    entries = [
        {
            "id": t["id"],
            "description": (t.get("name") or "")[len(MANUAL_MARK):].strip(),
            "minutes": t.get("estimated_time_min", 0),
            "created": t.get("completed"),
        }
        for t in upstream.tasks.values()
        if t.get("completed")
        and uid in (t.get("acked") or [])
        and (t.get("name") or "").startswith(MANUAL_MARK)
    ]
    entries.sort(key=lambda e: e["created"] or "", reverse=True)
    return {"entries": entries}


@router.post("/me/manual-work")
async def add_manual(body: ManualWorkIn, uid: str = Depends(require_uid)):
    """Record off-book work upstream (the documented pattern): create a chore,
    self-ack it, and mark it done — so the minutes land in the upstream stats and
    flow into the leaderboard/workload like any other completed chore."""
    task = None
    try:
        task = await upstream.create_task({
            "name": f"{MANUAL_MARK} {body.description}",
            "necessary_workers": 1,
            "estimated_time_min": body.minutes,
            "assignment_timeout_min": 15,
            "necessary_capabilities": None,
        })
        await upstream.ack(task["id"], uid)
        await upstream.mark_done(task["id"])
    except Exception as exc:  # noqa: BLE001
        if task:  # best-effort cleanup of a half-created task
            try:
                await upstream.delete_task(task["id"])
            except Exception:  # noqa: BLE001
                pass
        raise HTTPException(status_code=502, detail=f"Upstream log failed: {exc}")
    await service.broadcast_local({"type": "workload_updated", "discord_id": uid})
    return {"entry": {"id": task["id"], "description": body.description, "minutes": body.minutes}}


# --- reference data ---------------------------------------------------------

def _slugify(name: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-") or "chore"
    key = slug
    n = 2
    while store.template_key_exists(key):
        key = f"{slug}-{n}"
        n += 1
    return key


@router.get("/templates")
async def templates():
    return {"templates": store.all_templates()}


@router.post("/templates")
async def create_template(body: TemplateIn):
    key = _slugify(body.name)
    tpl = store.upsert_template(key, body.model_dump())
    return {"template": tpl}


@router.put("/templates/{key}")
async def update_template(key: str, body: TemplateIn):
    if not store.template_key_exists(key):
        raise HTTPException(status_code=404, detail="Unknown template")
    tpl = store.upsert_template(key, body.model_dump())
    return {"template": tpl}


@router.delete("/templates/{key}")
async def delete_template(key: str):
    if not store.template_key_exists(key):
        raise HTTPException(status_code=404, detail="Unknown template")
    store.delete_template(key)
    return {"ok": True}


@router.get("/skills")
async def skills():
    """Skill options for the chore/template capability pickers, sourced from the
    guild's `skill::` Discord roles (the upstream convention). Falls back to the
    capabilities present users report, then the built-in list for local dev."""
    caps = await fetch_skill_capabilities()
    if not caps:
        caps = sorted({c for u in upstream.users for c in (u.get("capabilities") or [])})
    return {"skills": caps or SKILLS}


@router.get("/stats")
async def stats():
    """Workload stats straight from upstream /stats (keyed by discord_id). No
    user/identity data — names are joined client-side against /api/users."""
    return {"stats": upstream.stats}


@router.get("/users")
async def users():
    """The trip roster straight from upstream /users, plus the children count so
    the dashboard can show total head-count context."""
    return {"users": upstream.users, "children_count": settings.CHILDREN_COUNT}


@router.get("/people")
async def people():
    pool = build_person_pool(upstream.users, upstream.stats)
    pool.sort(key=lambda p: (p["normalized_total"], p["workload_min"]))
    return {"people": pool, "children_count": settings.CHILDREN_COUNT}


@router.get("/leaderboard")
async def leaderboard():
    return {"rows": service.leaderboard()}


@router.get("/users/{user_id:path}")
async def user_detail(user_id: str):
    return await service.user_detail(user_id)


# --- chores -----------------------------------------------------------------

@router.get("/chores")
async def chores():
    views = service.list_chore_views()
    return {
        "chores": views,
        "suggestions": {c["id"]: service.suggestions_for(c["id"])["top"] for c in views},
        "upstream_connected": upstream.connected,
    }


@router.get("/chores/{task_id}")
async def get_chore(task_id: int):
    task = upstream.tasks.get(task_id)
    if not task:
        raise HTTPException(status_code=404, detail="Unknown chore")
    return {"chore": service.build_chore_view(task), "suggestions": service.suggestions_for(task_id)}


@router.get("/chores/{task_id}/suggestions")
async def chore_suggestions(task_id: int):
    return service.suggestions_for(task_id)


@router.post("/chores")
async def create_chore(body: ChoreCreateIn):
    template = store.get_template(body.template_key) if body.template_key else None

    # If created from a template, derive time (with head-count scaling) and caps.
    est = body.estimated_time_min
    caps = body.necessary_capabilities
    if template:
        headcount = body.headcount or 0
        est = template_time(template, headcount)
        caps = caps or template["necessary_capabilities"]

    # Urgency is encoded as 🌶️ in the name (upstream convention). Prepend the
    # requested peppers unless the author already typed some into the name.
    name = body.name
    if body.spiciness and spiciness_of(name) == 0:
        name = f"{SPICY * body.spiciness} {name}"

    upstream_body = {
        "name": name,
        "necessary_workers": body.necessary_workers,
        "estimated_time_min": est,
        "assignment_timeout_min": body.assignment_timeout_min,
        "necessary_capabilities": caps or None,
    }
    if body.deadline:
        upstream_body["deadline"] = body.deadline

    try:
        task = await upstream.create_task(upstream_body)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream create failed: {exc}")

    store.set_chore_meta(task["id"], size=size_for(est), template_key=body.template_key)
    view = service.build_chore_view(task)
    await service.broadcast_local(
        {"type": "task_created", "chore": view, "suggestions": service.suggestions_for(task["id"])["top"]}
    )
    return {"chore": view}


@router.post("/chores/{task_id}/claim")
async def claim_chore(task_id: int, uid: str = Depends(require_uid)):
    if task_id not in upstream.tasks:
        raise HTTPException(status_code=404, detail="Unknown chore")
    try:
        await upstream.ack(task_id, uid)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream claim failed: {exc}")
    view = service.build_chore_view(upstream.tasks[task_id])
    profile = store.get_profile(uid)
    name = (profile or {}).get("name", "friend")
    ack = random.choice(FUNNY_ACK_MESSAGES).format(name=name)
    await service.broadcast_local(
        {"type": "task_claimed", "chore": view, "by": uid,
         "suggestions": service.suggestions_for(task_id)["top"]}
    )
    return {"ack": ack, "chore": view}


@router.post("/chores/{task_id}/assign")
async def assign_chore(task_id: int, body: AssignIn):
    """Assign an unclaimed chore to someone else. With no discord_id, auto-assign
    the lowest-workload eligible person (the top suggestion)."""
    if task_id not in upstream.tasks:
        raise HTTPException(status_code=404, detail="Unknown chore")
    discord_id = body.discord_id
    if not discord_id:
        top = service.suggestions_for(task_id)["top"]
        if not top:
            raise HTTPException(status_code=409, detail="No eligible person available to auto-assign.")
        discord_id = top[0]
    try:
        await upstream.ack(task_id, discord_id)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream assign failed: {exc}")
    view = service.build_chore_view(upstream.tasks[task_id])
    name = service.person_name(discord_id)
    await service.broadcast_local(
        {"type": "task_claimed", "chore": view, "by": discord_id,
         "suggestions": service.suggestions_for(task_id)["top"]}
    )
    return {"chore": view, "assigned": {"discord_id": discord_id, "name": name}, "ack": f"Assigned to {name} ✓"}


@router.post("/chores/{task_id}/unclaim")
async def unclaim_chore(task_id: int, uid: str = Depends(require_uid)):
    try:
        await upstream.reject(task_id, uid)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream unclaim failed: {exc}")
    view = service.build_chore_view(upstream.tasks.get(task_id, {"id": task_id, "necessary_workers": 1}))
    await service.broadcast_local(
        {"type": "task_claimed", "chore": view, "by": uid,
         "suggestions": service.suggestions_for(task_id)["top"]}
    )
    return {"chore": view}


@router.post("/chores/{task_id}/unassign")
async def unassign_chore(task_id: int, body: AssignIn):
    """Remove a specific person's assignment from a chore (anyone can manage)."""
    if not body.discord_id:
        raise HTTPException(status_code=422, detail="discord_id is required")
    try:
        await upstream.reject(task_id, body.discord_id)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream unassign failed: {exc}")
    view = service.build_chore_view(upstream.tasks.get(task_id, {"id": task_id, "necessary_workers": 1}))
    await service.broadcast_local(
        {"type": "task_claimed", "chore": view, "by": body.discord_id,
         "suggestions": service.suggestions_for(task_id)["top"]}
    )
    return {"chore": view}


@router.post("/chores/{task_id}/time")
async def report_chore_time(task_id: int, body: TimeReportIn):
    """Override the time a person spent on a chore (upstream /tasks/{id}/time)."""
    try:
        await upstream.report_time(task_id, body.discord_id, body.time_spent_min)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream time report failed: {exc}")
    # The per-user aggregate (/stats) changes too, so refresh the cache now
    # rather than waiting for the 30s poll — keeps the leaderboard in sync.
    await upstream.refresh_stats()
    await service.broadcast_local({"type": "workload_updated", "discord_id": body.discord_id})
    return {"ok": True, "time_spent_min": body.time_spent_min}


@router.post("/chores/{task_id}/done")
async def done_chore(task_id: int):
    try:
        await upstream.mark_done(task_id)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream done failed: {exc}")
    await service.broadcast_local({"type": "task_done", "chore": {"id": task_id}})
    return {"ok": True}


@router.delete("/chores/{task_id}")
async def delete_chore(task_id: int):
    try:
        await upstream.delete_task(task_id)
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=502, detail=f"Upstream delete failed: {exc}")
    await service.broadcast_local({"type": "task_done", "chore": {"id": task_id}})
    return {"ok": True}
