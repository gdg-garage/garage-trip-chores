"""Fan-out WebSocket hub for our own clients (feed + dashboard).

Clients connect to /ws and receive JSON messages: an initial snapshot plus a
stream of {"type": ..., ...} events mirroring upstream state changes and local
actions (claims, profile/manual-work updates).
"""
from __future__ import annotations

import asyncio
import logging
from typing import Any

from fastapi import WebSocket

log = logging.getLogger("ws")


class ConnectionManager:
    def __init__(self) -> None:
        self._clients: set[WebSocket] = set()
        self._lock = asyncio.Lock()

    async def connect(self, ws: WebSocket) -> None:
        await ws.accept()
        async with self._lock:
            self._clients.add(ws)
        log.info("client connected (%d total)", len(self._clients))

    async def disconnect(self, ws: WebSocket) -> None:
        async with self._lock:
            self._clients.discard(ws)
        log.info("client disconnected (%d total)", len(self._clients))

    async def broadcast(self, message: dict[str, Any]) -> None:
        async with self._lock:
            targets = list(self._clients)
        dead = []
        for ws in targets:
            try:
                await ws.send_json(message)
            except Exception:  # noqa: BLE001
                dead.append(ws)
        if dead:
            async with self._lock:
                for ws in dead:
                    self._clients.discard(ws)


manager = ConnectionManager()
