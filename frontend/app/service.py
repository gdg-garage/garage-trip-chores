"""Service layer: merges upstream tasks with local metadata/claims into the
enriched "chore view" the UI consumes, computes suggestions, and bridges
upstream events to our own WebSocket fan-out.
"""
from __future__ import annotations

import asyncio
from datetime import datetime, timezone
from typing import Any, Optional

from . import store
from .catalog import size_for, spiciness_of
from .suggestions import build_person_pool, suggest
from .upstream import upstream
from .ws import manager

# A chore whose deadline is within this many minutes is treated as urgent even
# if it wasn't explicitly flagged.
URGENT_DEADLINE_MIN = 60


def _parse_dt(value: Optional[str]) -> Optional[datetime]:
    if not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def _is_active(task: dict[str, Any]) -> bool:
    return not task.get("completed") and not task.get("cancelled")


def build_chore_view(task: dict[str, Any]) -> dict[str, Any]:
    """Enrich a raw upstream task with local metadata, size, urgency, claims."""
    meta = store.get_chore_meta(task["id"]) or {}
    est = task.get("estimated_time_min", 0)
    size = meta.get("size") or size_for(est)

    # Urgency comes from 🌶️ in the name (upstream convention), or a near deadline.
    spiciness = spiciness_of(task.get("name") or "")
    urgent = spiciness > 0
    deadline = _parse_dt(task.get("deadline"))
    minutes_to_deadline = None
    if deadline:
        minutes_to_deadline = (deadline - datetime.now(timezone.utc)).total_seconds() / 60
        if minutes_to_deadline <= URGENT_DEADLINE_MIN:
            urgent = True

    claimers = task.get("acked") or []  # who's confirmed on it, from upstream
    directory = _person_directory()  # resolves both local profiles and upstream handles
    claimer_views = [
        {
            "discord_id": cid,
            "name": (directory.get(cid) or {}).get("name") or cid,
        }
        for cid in claimers
    ]

    return {
        "id": task["id"],
        "name": task.get("name"),
        "necessary_workers": task.get("necessary_workers", 1),
        "estimated_time_min": est,
        "assignment_timeout_min": task.get("assignment_timeout_min"),
        "necessary_capabilities": task.get("necessary_capabilities") or [],
        "deadline": task.get("deadline"),
        "minutes_to_deadline": round(minutes_to_deadline) if minutes_to_deadline is not None else None,
        "completed": task.get("completed"),
        "cancelled": task.get("cancelled"),
        "created": task.get("created"),
        "size": size,
        "urgent": urgent,
        "spiciness": spiciness,
        "template_key": meta.get("template_key"),
        "claimers": claimer_views,
        "claimed_count": len(claimer_views),
        "fully_claimed": len(claimer_views) >= task.get("necessary_workers", 1),
        "active": _is_active(task),
    }


def list_chore_views(active_only: bool = True) -> list[dict[str, Any]]:
    views = [build_chore_view(t) for t in upstream.tasks.values()]
    if active_only:
        views = [v for v in views if v["active"]]
    # urgent first, then soonest deadline, then largest first
    views.sort(
        key=lambda v: (
            not v["urgent"],
            v["minutes_to_deadline"] if v["minutes_to_deadline"] is not None else 10**9,
            -v["estimated_time_min"],
        )
    )
    return views


def suggestions_for(task_id: int) -> dict[str, Any]:
    task = upstream.tasks.get(task_id)
    if not task:
        return {"top": [], "ranked": []}
    pool = build_person_pool(upstream.users, upstream.stats)
    claimers = frozenset(task.get("acked") or [])
    fully_claimed = len(claimers) >= task.get("necessary_workers", 1)
    return suggest(task, pool, claimed_ids=claimers, fully_claimed=fully_claimed)


def _person_directory() -> dict[str, dict[str, str]]:
    """Union of upstream users and local profiles → {discord_id: {name, handle}}."""
    directory: dict[str, dict[str, str]] = {}
    for u in upstream.users:
        did = u.get("discord_id")
        if did:
            directory[did] = {"name": u.get("handle") or did, "handle": u.get("handle") or ""}
    for p in store.all_profiles():
        did = p["discord_id"]
        entry = directory.get(did, {})
        entry["name"] = p["name"]
        entry["handle"] = p.get("discord_handle") or entry.get("handle", "")
        directory[did] = entry
    return directory


def person_name(discord_id: str) -> str:
    info = _person_directory().get(discord_id)
    return (info or {}).get("name") or discord_id


def _claims_by_user() -> dict[str, list[int]]:
    per_user: dict[str, list[int]] = {}
    for task_id, task in upstream.tasks.items():
        for cid in (task.get("acked") or []):
            per_user.setdefault(cid, []).append(task_id)
    return per_user


def leaderboard() -> list[dict[str, Any]]:
    """One row per user from upstream /stats: chores done (worked_count) and time
    spent on them (worked_min), plus outstanding assignments. Names from /users;
    no local data."""
    names = {u["discord_id"]: u.get("handle") for u in upstream.users if u.get("discord_id")}
    rows: list[dict[str, Any]] = []
    for did, s in upstream.stats.items():
        rows.append(
            {
                "discord_id": did,
                "name": names.get(did) or did,
                "worked_count": int(s.get("worked_count", 0)),
                "worked_min": round(float(s.get("worked_min", 0)), 1),
                "assigned_count": int(s.get("assigned_count", 0)),
            }
        )
    # default order: most time worked first
    rows.sort(key=lambda r: (-r["worked_min"], -r["worked_count"], r["name"].lower()))
    return rows


async def _time_spent(task_id: int, fallback: int) -> int:
    """Actual time spent on a task from GET /tasks/{id}/stats (total_time_min),
    falling back to the estimate if the stats call fails or is empty."""
    try:
        stats = await upstream.task_stats(task_id)
    except Exception:  # noqa: BLE001
        return fallback
    val = stats.get("total_time_min")
    return val if val is not None else fallback


async def user_detail(discord_id: str) -> dict[str, Any]:
    """A person's chores: those in progress (on top) and those completed.
    Completed chores carry `total_time_min` — the real time spent, from
    /tasks/{id}/stats — not just the estimate."""
    directory = _person_directory()
    info = directory.get(discord_id, {"name": discord_id, "handle": ""})
    task_ids = _claims_by_user().get(discord_id, [])

    performing: list[dict[str, Any]] = []
    performed: list[dict[str, Any]] = []
    for task_id in task_ids:
        task = upstream.tasks.get(task_id)
        if not task or task.get("cancelled"):
            continue
        view = build_chore_view(task)
        if task.get("completed"):
            view["total_time_min"] = await _time_spent(task_id, view["estimated_time_min"])
            performed.append(view)
        else:
            performing.append(view)

    performing.sort(key=lambda v: (not v["urgent"], v.get("created") or ""))
    performed.sort(key=lambda v: v.get("completed") or "", reverse=True)

    return {
        "discord_id": discord_id,
        "name": info.get("name") or discord_id,
        "handle": info.get("handle") or "",
        "performing": performing,
        "performed": performed,
        "time_spent_min": sum(v["total_time_min"] for v in performed),
    }


def snapshot() -> dict[str, Any]:
    """Full initial payload sent to a client right after it connects."""
    chores = list_chore_views()
    return {
        "type": "snapshot",
        "chores": chores,
        "suggestions": {c["id"]: suggestions_for(c["id"])["top"] for c in chores},
        "upstream_connected": upstream.connected,
    }


async def on_upstream_event(event: dict[str, Any]) -> None:
    """Handler registered with the upstream client; re-broadcast enriched."""
    chore = event.get("chore")
    payload: dict[str, Any] = {"type": event.get("type")}
    if chore and isinstance(chore, dict) and "id" in chore:
        # Prefer the cached task: the upstream client re-fetches it on assignment
        # events, so its `acked` array (who's on it) is current.
        task = upstream.tasks.get(chore["id"], chore)
        view = build_chore_view(task)
        payload["chore"] = view
        payload["suggestions"] = suggestions_for(chore["id"])["top"]
    payload["assignment"] = event.get("assignment")
    await manager.broadcast(payload)


async def broadcast_local(message: dict[str, Any]) -> None:
    """Broadcast a locally-originated change (claim, profile edit, etc.)."""
    await manager.broadcast(message)


def _chore_signature(task: dict[str, Any]) -> tuple:
    """A cheap fingerprint of the fields that, when changed, should be pushed to
    clients — so the reconcile poll only broadcasts on real changes."""
    return (
        _is_active(task),
        task.get("name"),
        task.get("completed"),
        task.get("cancelled"),
        task.get("necessary_workers"),
        task.get("estimated_time_min"),
        task.get("deadline"),
        tuple(task.get("acked") or []),
    )


async def reconcile_and_broadcast() -> None:
    """Periodic safety net: pull the upstream truth and push out anything the
    live WebSocket stream may have missed — e.g. a chore completed or claimed via
    Discord or directly over the API. The upstream is the source of truth."""
    before = {tid: _chore_signature(t) for tid, t in upstream.tasks.items()}
    await asyncio.gather(
        upstream.refresh_tasks(), upstream.refresh_users(), upstream.refresh_stats()
    )
    after = upstream.tasks

    # Chores that disappeared or are no longer active → tell clients they're off
    # the board (same shape the app emits when marking done itself).
    for tid, sig in before.items():
        task = after.get(tid)
        if (task is None or not _is_active(task)) and sig[0]:  # sig[0] == was active
            await broadcast_local({"type": "task_done", "chore": {"id": tid}})

    # New or changed active chores → push a fresh enriched view.
    for tid, task in after.items():
        if not _is_active(task):
            continue
        if before.get(tid) != _chore_signature(task):
            await broadcast_local({
                "type": "task_updated",
                "chore": build_chore_view(task),
                "suggestions": suggestions_for(tid)["top"],
            })
