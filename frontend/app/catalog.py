"""Static catalog: the preset chore templates and the pool of funny
acknowledgement messages shown when a user claims a chore.
"""
from __future__ import annotations

from typing import Any

# Canonical expertise/skill strings used as `necessary_capabilities`.
SKILLS = ["cooking", "grilling", "hookah_master", "cleaning", "driving"]

# Urgency convention (upstream garage-trip-chores): 1–3 🌶️ in the chore *name*
# flag priority. The name is the source of truth, so chores made via Discord or
# the API with peppers are recognised too.
SPICY = "🌶️"
MAX_SPICY = 3

# Prefixes the name of an off-book "manual work" chore (logged via create →
# self-ack → done), so those entries are identifiable in the person's history.
MANUAL_MARK = "📝"
_PEPPER = "\U0001f336"  # base codepoint, matched with or without the variation selector


def spiciness_of(name: str) -> int:
    """How many 🌶️ are in a chore name (capped at MAX_SPICY)."""
    return min((name or "").count(_PEPPER), MAX_SPICY)

# Preset chores from the spec. `scales_with_headcount` chores get longer the
# more mouths there are to feed / dishes to wash (children included).
CHORE_TEMPLATES: list[dict[str, Any]] = [
    {
        "key": "floor-sweep-dry",
        "name": "Floor sweep (dry)",
        "necessary_workers": 1,
        "estimated_time_min": 15,
        "assignment_timeout_min": 15,
        "necessary_capabilities": [],
        "scales_with_headcount": False,
        "per_person_min": 0,
    },
    {
        "key": "floor-sweep-water",
        "name": "Floor sweep (with water)",
        "necessary_workers": 1,
        "estimated_time_min": 25,
        "assignment_timeout_min": 15,
        "necessary_capabilities": [],
        "scales_with_headcount": False,
        "per_person_min": 0,
    },
    {
        "key": "dishwasher-load",
        "name": "Load the dishwasher",
        "necessary_workers": 1,
        "estimated_time_min": 10,
        "assignment_timeout_min": 15,
        "necessary_capabilities": [],
        "scales_with_headcount": True,
        "per_person_min": 1,
    },
    {
        "key": "dishwasher-unload",
        "name": "Unload the dishwasher",
        "necessary_workers": 1,
        "estimated_time_min": 8,
        "assignment_timeout_min": 15,
        "necessary_capabilities": [],
        "scales_with_headcount": True,
        "per_person_min": 1,
    },
    {
        "key": "take-out-bin",
        "name": "Take out the bin",
        "necessary_workers": 1,
        "estimated_time_min": 5,
        "assignment_timeout_min": 10,
        "necessary_capabilities": [],
        "scales_with_headcount": False,
        "per_person_min": 0,
    },
    {
        "key": "grilling",
        "name": "Grilling",
        "necessary_workers": 2,
        "estimated_time_min": 60,
        "assignment_timeout_min": 20,
        "necessary_capabilities": ["grilling"],
        "scales_with_headcount": True,
        "per_person_min": 3,
    },
    {
        "key": "kitchen-sweep",
        "name": "Kitchen sweep",
        "necessary_workers": 1,
        "estimated_time_min": 20,
        "assignment_timeout_min": 15,
        "necessary_capabilities": [],
        "scales_with_headcount": False,
        "per_person_min": 0,
    },
    {
        "key": "water-pipe-cleaning",
        "name": "Water pipe (shisha) cleaning",
        "necessary_workers": 1,
        "estimated_time_min": 45,
        "assignment_timeout_min": 20,
        "necessary_capabilities": ["hookah_master"],
        "scales_with_headcount": False,
        "per_person_min": 0,
    },
    {
        "key": "cooking",
        "name": "Cooking",
        "necessary_workers": 2,
        "estimated_time_min": 90,
        "assignment_timeout_min": 20,
        "necessary_capabilities": ["cooking"],
        "scales_with_headcount": True,
        "per_person_min": 4,
    },
]

TEMPLATES_BY_KEY = {t["key"]: t for t in CHORE_TEMPLATES}


def template_time(template: dict[str, Any], headcount: int | None) -> int:
    """Estimated minutes for a template, scaled by head-count when relevant."""
    base = template["estimated_time_min"]
    if template.get("scales_with_headcount") and headcount:
        return base + template.get("per_person_min", 0) * headcount
    return base


def size_for(estimated_time_min: int) -> str:
    """Bucket a chore into a size label from its estimated time."""
    if estimated_time_min <= 10:
        return "small"
    if estimated_time_min <= 30:
        return "medium"
    return "large"


FUNNY_ACK_MESSAGES = [
    "🦸 Chore hero incoming! Thanks, {name}!",
    "🎉 {name} said yes to the mess!",
    "🧹 {name} is on it like a bonnet!",
    "💪 Absolute legend, {name}. The dishes tremble.",
    "🚀 {name} launched into action!",
    "🏆 Garage Trip MVP: {name}!",
    "🔥 {name} grabbed it before anyone else could blink.",
    "🧽 Scrub-a-dub, {name} to the rescue!",
    "🥇 {name} just earned some serious chore cred.",
    "😎 Cool, calm, and cleaning: that's {name}.",
    "🎯 {name} claimed it. Bullseye.",
    "🙌 The mountain thanks you, {name}!",
    "⚡ Lightning-fast {name} strikes again.",
    "🐝 Busy as a bee, {name} buzzes off to work.",
    "🎈 Party's over, chore's on — go {name}!",
]
