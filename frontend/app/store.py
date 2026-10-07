"""Persistence abstraction.

The app talks to `store` for all local data; the concrete backend is chosen by
`STORE_BACKEND`. Today only "sqlite" (app/db.py) exists. The "api" backend
arrives in Phase 3 and must implement this same set of names, so call sites
never change.
"""
from __future__ import annotations

from .config import settings

if settings.STORE_BACKEND == "sqlite":
    from . import db as _backend
else:  # pragma: no cover - api backend lands in Phase 3
    raise RuntimeError(
        f"Unknown STORE_BACKEND {settings.STORE_BACKEND!r}; only 'sqlite' is available so far."
    )

# --- lifecycle ---
init_db = _backend.init_db

# --- profiles ---
upsert_profile = _backend.upsert_profile
get_profile = _backend.get_profile
get_profile_by_handle = _backend.get_profile_by_handle
all_profiles = _backend.all_profiles

# --- chore metadata ---
set_chore_meta = _backend.set_chore_meta
get_chore_meta = _backend.get_chore_meta
all_chore_meta = _backend.all_chore_meta

# --- templates ---
all_templates = _backend.all_templates
get_template = _backend.get_template
upsert_template = _backend.upsert_template
delete_template = _backend.delete_template
template_key_exists = _backend.template_key_exists

# --- merge ---
merge_user = _backend.merge_user
