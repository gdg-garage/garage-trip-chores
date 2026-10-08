"""Ranking of who should take a chore.

Per the agreed design:
  * Required skill is a HARD eligibility filter — someone who lacks a required
    capability is not suggested at all.
  * Among the eligible, we rank by LOWEST current workload so work stays
    balanced. The top 3 are the "notified" people (highlighted / sound on their
    screens).
"""
from __future__ import annotations

from typing import Any

from .upstream import upstream


def _committed_minutes() -> dict[str, float]:
    """Minutes each person has already committed to via in-progress chores they've
    claimed (acked) upstream but not yet completed (so not in the upstream stats
    yet), so auto-assign accounts for work someone is already on the hook for."""
    committed: dict[str, float] = {}
    for task in upstream.tasks.values():
        if not task or task.get("completed") or task.get("cancelled"):
            continue
        est = task.get("estimated_time_min", 0)
        for cid in (task.get("acked") or []):
            committed[cid] = committed.get(cid, 0) + est
    return committed


def build_person_pool(
    users: list[dict[str, Any]],
    stats: dict[str, dict[str, Any]],
) -> list[dict[str, Any]]:
    """Build the suggestion pool purely from the upstream API: present users
    (`/users`) with their Discord-role capabilities, ranked by workload from
    `/stats` plus in-progress (acked) claims not yet in the stats.

    `/users` returns only present users, so people who left simply aren't there."""
    committed = _committed_minutes()
    pool: list[dict[str, Any]] = []

    for u in users:
        did = u.get("discord_id")
        if not did:
            continue
        s = stats.get(did, {})
        worked_min = float(s.get("total_min", 0)) + committed.get(did, 0)
        pool.append(
            {
                "discord_id": did,
                "name": u.get("handle") or did,
                "handle": u.get("handle") or "",
                "capabilities": sorted(set(u.get("capabilities") or [])),
                "workload_min": round(worked_min, 1),
                "normalized_total": float(s.get("normalized_total", 0)),
                "present_ticks": int(s.get("present_ticks", 0)),
            }
        )
    return pool


def _eligible(person: dict[str, Any], required_caps: list[str]) -> bool:
    # skill gate: must have every required capability
    return not required_caps or set(required_caps).issubset(set(person["capabilities"]))


def suggest(
    task: dict[str, Any],
    pool: list[dict[str, Any]],
    top_n: int = 3,
    claimed_ids: frozenset[str] = frozenset(),
    fully_claimed: bool = False,
) -> dict[str, Any]:
    """Return ranked suggestions for a task.

    Result: {"top": [...top_n discord_ids...], "ranked": [person, ...]}.
    Each person is annotated with `eligible` and `suggested` flags.

    People already on the chore (`claimed_ids`) are never in `top`, and a
    `fully_claimed` chore suggests nobody (all worker slots are filled).
    """
    required_caps = task.get("necessary_capabilities") or []

    annotated = []
    for p in pool:
        p = dict(p)
        p["eligible"] = _eligible(p, required_caps)
        annotated.append(p)

    eligible = [p for p in annotated if p["eligible"]]
    # rank: least total workload first (upstream + manual + in-progress claims),
    # then the upstream normalized metric, then name
    eligible.sort(key=lambda p: (p["workload_min"], p["normalized_total"], p["name"].lower()))

    # Don't suggest people already on the chore, and suggest nobody once it's
    # fully staffed.
    if fully_claimed:
        top_ids: list[str] = []
    else:
        top_ids = [p["discord_id"] for p in eligible if p["discord_id"] not in claimed_ids][:top_n]
    for p in annotated:
        p["suggested"] = p["discord_id"] in top_ids

    # ranked list: eligible (sorted) first, then the rest for reference
    rest = [p for p in annotated if not p["eligible"]]
    return {"top": top_ids, "ranked": eligible + rest}
