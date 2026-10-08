"""FastAPI application entrypoint for the Garage Trip Chores app."""
from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager
from pathlib import Path

from typing import Optional

import httpx
import websockets
from fastapi import FastAPI, Request, Response, WebSocket, WebSocketDisconnect
from fastapi.responses import JSONResponse, RedirectResponse
from fastapi.staticfiles import StaticFiles
from starlette.middleware.sessions import SessionMiddleware

from . import service, store
from .auth import router as auth_router
from .config import settings
from .routes import api as api_routes
from .routes import pages as page_routes
from .upstream import upstream
from .ws import manager

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(levelname)s %(message)s")
log = logging.getLogger("app")

STATIC_DIR = Path(__file__).resolve().parent / "static"


class RevalidatingStatic(StaticFiles):
    """Serve static assets with `Cache-Control: no-cache` so browsers always
    revalidate (cheap 304 when unchanged) and pick up new JS/CSS immediately
    after a deploy — avoids stale bundles rendering against a changed API."""

    async def get_response(self, path, scope):
        response = await super().get_response(path, scope)
        response.headers["Cache-Control"] = "no-cache"
        return response


async def _poll_loop() -> None:
    """Fallback reconcile so tasks/stats/users stay coherent — and connected
    clients stay updated — even if the upstream WebSocket is quiet or an event is
    missed (e.g. a chore completed or claimed via Discord or directly over the
    API). Diffs the refreshed upstream state and broadcasts what changed."""
    while True:
        await asyncio.sleep(30)
        await service.reconcile_and_broadcast()


async def _heartbeat_loop() -> None:
    """Ping connected clients so idle WebSockets stay alive and dead ones are
    pruned — keeps the always-on TV dashboard receiving live updates."""
    while True:
        await asyncio.sleep(25)
        await manager.broadcast({"type": "ping"})


@asynccontextmanager
async def lifespan(app: FastAPI):
    store.init_db()
    upstream.on_event(service.on_upstream_event)
    if not settings.has_upstream_key:
        log.warning("CHORES_API_KEY is empty — set it in .env for live upstream sync.")
    upstream.start()
    # prime caches immediately (WS also primes on connect)
    await asyncio.gather(upstream.refresh_tasks(), upstream.refresh_users(), upstream.refresh_stats())
    poller = asyncio.create_task(_poll_loop())
    heartbeat = asyncio.create_task(_heartbeat_loop())
    try:
        yield
    finally:
        poller.cancel()
        heartbeat.cancel()
        await upstream.close()
        if _proxy_http and not _proxy_http.is_closed:
            await _proxy_http.aclose()


app = FastAPI(title="Garage Trip Chores", version="3.0.0", docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)

# Paths reachable without a session (so people can actually log in).
_PUBLIC_EXACT = {"/", "/healthz", "/health", "/unauthorized", "/favicon.ico", "/openapi.json", "/openapi.yaml", "/summary"}
_PUBLIC_PREFIXES = (
    "/auth/",
    "/static/",
    "/tasks",
    "/stats",
    "/skills",
    "/docs",
    "/schemas",
    "/api/tasks",
    "/api/health",
    "/api/docs",
    "/api/openapi",
    "/api/schemas",
    "/api/summary",
)


def _is_public(path: str) -> bool:
    if path in _PUBLIC_EXACT or path == "/users":
        return True
    return path.startswith(_PUBLIC_PREFIXES)


def _session_authed(session) -> bool:
    return bool(session.get("user") or session.get("tablet"))


def _is_tv_host(request: Request) -> bool:
    return (request.url.hostname or "").startswith("tv.")


# Auth gate. Defined before SessionMiddleware is added so that middleware ends
# up OUTER (runs first) and `request.session` is populated here.
@app.middleware("http")
async def require_auth(request: Request, call_next):
    if _is_tv_host(request):
        # TV subdomain: read-only dashboard display, no login required.
        if request.url.path == "/":
            return RedirectResponse("/dashboard", status_code=302)
        return await call_next(request)
    if settings.AUTH_REQUIRED and not _is_public(request.url.path):
        if not _session_authed(request.session):
            if request.url.path.startswith("/api"):
                return JSONResponse({"detail": "Login required"}, status_code=401)
            return RedirectResponse("/", status_code=302)
    return await call_next(request)


app.add_middleware(SessionMiddleware, secret_key=settings.SESSION_SECRET, same_site="lax")
app.mount("/static", RevalidatingStatic(directory=str(STATIC_DIR)), name="static")
app.include_router(auth_router)
app.include_router(page_routes.router)
app.include_router(api_routes.router)


# --- Upstream Go API Proxy (enables single-container serving) ---
_proxy_http: Optional[httpx.AsyncClient] = None


def _get_proxy_http() -> httpx.AsyncClient:
    global _proxy_http
    if _proxy_http is None or _proxy_http.is_closed:
        _proxy_http = httpx.AsyncClient(base_url=settings.CHORES_API_BASE, timeout=30.0)
    return _proxy_http


async def _proxy_to_upstream(request: Request, target_path: Optional[str] = None) -> Response:
    client = _get_proxy_http()
    path = target_path if target_path is not None else request.url.path
    if path.startswith("/api/") and not path.startswith("/api/ws"):
        path = path[4:]  # strip /api for Go endpoints
    query = request.url.query.encode("utf-8") if request.url.query else None
    url = httpx.URL(path=path, query=query)

    headers = dict(request.headers)
    headers.pop("host", None)
    headers.pop("content-length", None)
    body = await request.body()

    try:
        resp = await client.request(
            method=request.method,
            url=url,
            headers=headers,
            content=body,
        )
        resp_headers = dict(resp.headers)
        resp_headers.pop("content-encoding", None)
        resp_headers.pop("content-length", None)
        resp_headers.pop("transfer-encoding", None)
        return Response(content=resp.content, status_code=resp.status_code, headers=resp_headers)
    except Exception as exc:
        log.warning("Upstream proxy request failed (%s %s): %s", request.method, path, exc)
        return JSONResponse({"detail": f"Upstream service error: {exc}"}, status_code=502)


# Upstream API routes proxied to Go backend
@app.api_route("/tasks{path:path}", methods=["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"], include_in_schema=False)
@app.api_route("/stats{path:path}", methods=["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"], include_in_schema=False)
@app.api_route("/skills", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/health", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/docs{path:path}", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/openapi.json", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/openapi.yaml", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/schemas{path:path}", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/summary", methods=["POST", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/tasks{path:path}", methods=["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/health", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/docs{path:path}", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/openapi.json", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/openapi.yaml", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/schemas{path:path}", methods=["GET", "OPTIONS"], include_in_schema=False)
@app.api_route("/api/summary", methods=["POST", "OPTIONS"], include_in_schema=False)
async def proxy_upstream_endpoint(request: Request):
    return await _proxy_to_upstream(request)


@app.get("/users", include_in_schema=False)
async def proxy_users_list(request: Request):
    return await _proxy_to_upstream(request, target_path="/users")


@app.websocket("/api/ws")
async def proxy_api_ws_endpoint(client_ws: WebSocket):
    """Proxy WebSocket connection to Go upstream WebSocket (/ws)."""
    await client_ws.accept()
    go_ws_url = settings.CHORES_WS_URL
    try:
        async with websockets.connect(go_ws_url) as server_ws:
            async def forward_to_client():
                async for msg in server_ws:
                    await client_ws.send_text(msg if isinstance(msg, str) else msg.decode("utf-8"))

            async def forward_to_server():
                while True:
                    text = await client_ws.receive_text()
                    await server_ws.send(text)

            t1 = asyncio.create_task(forward_to_client())
            t2 = asyncio.create_task(forward_to_server())
            done, pending = await asyncio.wait([t1, t2], return_when=asyncio.FIRST_COMPLETED)
            for t in pending:
                t.cancel()
    except Exception as exc:
        log.debug("Proxy /api/ws closed: %s", exc)
    finally:
        try:
            await client_ws.close()
        except Exception:
            pass


@app.websocket("/ws")
async def ws_endpoint(ws: WebSocket):
    if settings.AUTH_REQUIRED and not _session_authed(ws.session):
        await ws.close(code=1008)  # policy violation
        return
    await manager.connect(ws)
    try:
        await ws.send_json(service.snapshot())
        while True:
            # We don't require anything from clients; just keep the socket open.
            await ws.receive_text()
    except WebSocketDisconnect:
        await manager.disconnect(ws)
    except Exception:  # noqa: BLE001
        await manager.disconnect(ws)


@app.get("/healthz")
async def healthz():
    return {"status": "ok", "upstream_connected": upstream.connected}
