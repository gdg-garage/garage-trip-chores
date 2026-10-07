"""SQLite persistence for the data the upstream chores API does not hold:
local profiles (name/handle cache), per-chore metadata (size, template link),
and chore templates.

The upstream API is the source of truth for tasks/assignments/stats (claims are
its `acked` arrays; off-book work is logged as completed tasks); this layer only
augments it. Scale is tiny (~15 users), so a single shared connection guarded by
a lock is plenty.
"""
from __future__ import annotations

import json
import sqlite3
import threading
from datetime import datetime, timezone
from typing import Any, Optional

from .config import settings

_lock = threading.Lock()
_conn: Optional[sqlite3.Connection] = None


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


def get_conn() -> sqlite3.Connection:
    global _conn
    if _conn is None:
        _conn = sqlite3.connect(settings.DB_PATH, check_same_thread=False)
        _conn.row_factory = sqlite3.Row
        _conn.execute("PRAGMA journal_mode=WAL")
    return _conn


SCHEMA = """
CREATE TABLE IF NOT EXISTS profiles (
    discord_id      TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    discord_handle  TEXT NOT NULL,
    created         TEXT NOT NULL,
    updated         TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS chore_meta (
    task_id      INTEGER PRIMARY KEY,
    size         TEXT NOT NULL DEFAULT 'medium',  -- small|medium|large (derived, overridable)
    template_key TEXT
);

CREATE TABLE IF NOT EXISTS templates (
    key                    TEXT PRIMARY KEY,
    name                   TEXT NOT NULL,
    necessary_workers      INTEGER NOT NULL DEFAULT 1,
    estimated_time_min     INTEGER NOT NULL DEFAULT 10,
    assignment_timeout_min INTEGER NOT NULL DEFAULT 15,
    necessary_capabilities TEXT NOT NULL DEFAULT '[]',  -- json array
    scales_with_headcount  INTEGER NOT NULL DEFAULT 0,
    per_person_min         INTEGER NOT NULL DEFAULT 0,
    sort_order             INTEGER NOT NULL DEFAULT 0
);
"""


def init_db() -> None:
    with _lock:
        conn = get_conn()
        conn.executescript(SCHEMA)
        conn.commit()
    _seed_templates()


def _seed_templates() -> None:
    """Populate the templates table from the built-in defaults on first run."""
    from .catalog import CHORE_TEMPLATES

    with _lock:
        conn = get_conn()
        if conn.execute("SELECT COUNT(*) FROM templates").fetchone()[0] > 0:
            return
        for i, t in enumerate(CHORE_TEMPLATES):
            conn.execute(
                """
                INSERT INTO templates (key, name, necessary_workers, estimated_time_min,
                    assignment_timeout_min, necessary_capabilities, scales_with_headcount,
                    per_person_min, sort_order)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    t["key"], t["name"], t["necessary_workers"], t["estimated_time_min"],
                    t["assignment_timeout_min"], json.dumps(t["necessary_capabilities"]),
                    1 if t.get("scales_with_headcount") else 0, t.get("per_person_min", 0), i,
                ),
            )
        conn.commit()


# --- profiles ---------------------------------------------------------------

def upsert_profile(
    discord_id: str,
    name: str,
    discord_handle: str,
) -> dict[str, Any]:
    with _lock:
        conn = get_conn()
        now = _now()
        conn.execute(
            """
            INSERT INTO profiles (discord_id, name, discord_handle, created, updated)
            VALUES (?, ?, ?, ?, ?)
            ON CONFLICT(discord_id) DO UPDATE SET
                name=excluded.name,
                discord_handle=excluded.discord_handle,
                updated=excluded.updated
            """,
            (discord_id, name, discord_handle, now, now),
        )
        conn.commit()
    return get_profile(discord_id)  # type: ignore[return-value]


def get_profile(discord_id: str) -> Optional[dict[str, Any]]:
    with _lock:
        row = get_conn().execute(
            "SELECT * FROM profiles WHERE discord_id = ?", (discord_id,)
        ).fetchone()
    return _profile_row(row) if row else None


def get_profile_by_handle(handle: str) -> Optional[dict[str, Any]]:
    with _lock:
        row = get_conn().execute(
            "SELECT * FROM profiles WHERE lower(discord_handle) = lower(?)", (handle,)
        ).fetchone()
    return _profile_row(row) if row else None


def all_profiles() -> list[dict[str, Any]]:
    with _lock:
        rows = get_conn().execute("SELECT * FROM profiles").fetchall()
    return [_profile_row(r) for r in rows]


def _profile_row(row: sqlite3.Row) -> dict[str, Any]:
    return dict(row)


# --- chore metadata ---------------------------------------------------------

def set_chore_meta(task_id: int, size: str, template_key: Optional[str]) -> None:
    with _lock:
        conn = get_conn()
        conn.execute(
            """
            INSERT INTO chore_meta (task_id, size, template_key)
            VALUES (?, ?, ?)
            ON CONFLICT(task_id) DO UPDATE SET
                size=excluded.size, template_key=excluded.template_key
            """,
            (task_id, size, template_key),
        )
        conn.commit()


def get_chore_meta(task_id: int) -> Optional[dict[str, Any]]:
    with _lock:
        row = get_conn().execute(
            "SELECT * FROM chore_meta WHERE task_id = ?", (task_id,)
        ).fetchone()
    return dict(row) if row else None


def all_chore_meta() -> dict[int, dict[str, Any]]:
    with _lock:
        rows = get_conn().execute("SELECT * FROM chore_meta").fetchall()
    return {r["task_id"]: dict(r) for r in rows}


# --- merge two identities ----------------------------------------------------

def merge_user(from_id: str, to_id: str) -> dict[str, int]:
    """Fold `from_id` into `to_id` by deleting the duplicate profile. Returns the
    number of profiles removed. (Claims and off-book work live upstream now,
    keyed by discord_id, so they aren't touched here.)"""
    with _lock:
        conn = get_conn()
        removed = conn.execute(
            "DELETE FROM profiles WHERE discord_id = ?", (from_id,)
        ).rowcount
        conn.commit()
    return {"profiles": removed}


# --- chore templates --------------------------------------------------------

def _template_row(row: sqlite3.Row) -> dict[str, Any]:
    d = dict(row)
    d["necessary_capabilities"] = json.loads(d.get("necessary_capabilities") or "[]")
    d["scales_with_headcount"] = bool(d["scales_with_headcount"])
    return d


def all_templates() -> list[dict[str, Any]]:
    with _lock:
        rows = get_conn().execute(
            "SELECT * FROM templates ORDER BY sort_order, name"
        ).fetchall()
    return [_template_row(r) for r in rows]


def get_template(key: str) -> Optional[dict[str, Any]]:
    with _lock:
        row = get_conn().execute("SELECT * FROM templates WHERE key = ?", (key,)).fetchone()
    return _template_row(row) if row else None


def upsert_template(key: str, data: dict[str, Any]) -> dict[str, Any]:
    with _lock:
        conn = get_conn()
        # keep new templates after the existing ones
        next_order = conn.execute("SELECT COALESCE(MAX(sort_order), -1) + 1 FROM templates").fetchone()[0]
        conn.execute(
            """
            INSERT INTO templates (key, name, necessary_workers, estimated_time_min,
                assignment_timeout_min, necessary_capabilities, scales_with_headcount,
                per_person_min, sort_order)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(key) DO UPDATE SET
                name=excluded.name,
                necessary_workers=excluded.necessary_workers,
                estimated_time_min=excluded.estimated_time_min,
                assignment_timeout_min=excluded.assignment_timeout_min,
                necessary_capabilities=excluded.necessary_capabilities,
                scales_with_headcount=excluded.scales_with_headcount,
                per_person_min=excluded.per_person_min
            """,
            (
                key, data["name"], data["necessary_workers"], data["estimated_time_min"],
                data["assignment_timeout_min"], json.dumps(data["necessary_capabilities"]),
                1 if data["scales_with_headcount"] else 0, data["per_person_min"], next_order,
            ),
        )
        conn.commit()
    return get_template(key)  # type: ignore[return-value]


def delete_template(key: str) -> None:
    with _lock:
        conn = get_conn()
        conn.execute("DELETE FROM templates WHERE key = ?", (key,))
        conn.commit()


def template_key_exists(key: str) -> bool:
    with _lock:
        return get_conn().execute(
            "SELECT 1 FROM templates WHERE key = ?", (key,)
        ).fetchone() is not None
