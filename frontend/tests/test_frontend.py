import pytest
from datetime import datetime, timezone, timedelta
from app.models import ChoreCreateIn, ManualWorkIn
from app import service
from app.main import _is_public, app


def test_chore_create_model():
    # Normal chore
    m = ChoreCreateIn(name="Wash dishes")
    assert m.delay_min == 0
    assert not m.self_reported
    assert m.creator_id is None
    assert m.assignee_id is None

    # Delayed chore
    m_delayed = ChoreCreateIn(name="Empty trash", delay_min=60)
    assert m_delayed.delay_min == 60

    # Self reported chore
    m_self = ChoreCreateIn(name="Cleaned kitchen", self_reported=True, creator_id="user123")
    assert m_self.self_reported is True
    assert m_self.creator_id == "user123"

    # Direct assignee
    m_assign = ChoreCreateIn(name="Cook dinner", assignee_id="chef456")
    assert m_assign.assignee_id == "chef456"


def test_build_chore_view_delayed_and_self_reported():
    from app import store
    store.init_db()
    now = datetime.now(timezone.utc)
    future = (now + timedelta(minutes=45)).isoformat()

    task_delayed = {
        "id": 101,
        "name": "Delayed task",
        "estimated_time_min": 20,
        "delay_min": 45,
        "publish_at": future,
        "self_reported": False,
        "worklogs": [],
    }
    view = service.build_chore_view(task_delayed)
    assert view["id"] == 101
    assert view["delay_min"] == 45
    assert view["is_delayed"] is True
    assert view["minutes_to_publish"] is not None
    assert 40 <= view["minutes_to_publish"] <= 46
    assert view["self_reported"] is False

    task_self_reported = {
        "id": 102,
        "name": "Self reported task",
        "estimated_time_min": 15,
        "self_reported": True,
        "creator_id": "worker99",
        "completed": now.isoformat(),
        "worklogs": [{"user_id": "worker99", "time_spent_min": 15, "self_reported": True}],
        "worked_min_total": 15,
    }
    view_self = service.build_chore_view(task_self_reported)
    assert view_self["id"] == 102
    assert view_self["self_reported"] is True
    assert view_self["creator_id"] == "worker99"
    assert len(view_self["worklogs"]) == 1
    assert view_self["worked_min_total"] == 15
    assert view_self["is_delayed"] is False


def test_is_public_routing():
    # Public pages
    assert _is_public("/")
    assert _is_public("/healthz")
    assert _is_public("/health")
    assert _is_public("/favicon.ico")
    assert _is_public("/auth/discord")
    assert _is_public("/static/css/app.css")

    # Proxied Go API routes must bypass session cookie auth so external Bearer clients work
    assert _is_public("/tasks")
    assert _is_public("/tasks/1")
    assert _is_public("/tasks/1/done")
    assert _is_public("/stats")
    assert _is_public("/skills")
    assert _is_public("/users")
    assert _is_public("/docs")
    assert _is_public("/openapi.json")
    assert _is_public("/api/tasks")
    assert _is_public("/api/health")

    # Protected frontend app pages require auth when AUTH_REQUIRED=True
    assert not _is_public("/feed")
    assert not _is_public("/profile")
    assert not _is_public("/manage")
    assert not _is_public("/dashboard")
    assert not _is_public("/api/me")
    assert not _is_public("/api/chores")


def test_api_routes():
    from starlette.testclient import TestClient
    from app import store, upstream

    store.init_db()
    client = TestClient(app)

    # Health check
    res = client.get("/healthz")
    assert res.status_code == 200
    assert res.json()["status"] == "ok"

    # Templates
    res = client.get("/api/templates")
    assert res.status_code == 200
    assert "templates" in res.json()

    # Skills endpoint
    res = client.get("/api/skills")
    assert res.status_code == 200
    assert "skills" in res.json()
    assert len(res.json()["skills"]) > 0

