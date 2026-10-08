#!/bin/bash
set -e

# Ensure persistent data directory exists
mkdir -p /app/data

# Internal port for Go API backend
export CHORES_API_PORT="${CHORES_API_PORT:-8081}"
export CHORES_API_HOST="${CHORES_API_HOST:-127.0.0.1}"

# Python frontend connects to Go API on localhost
export CHORES_API_BASE="http://127.0.0.1:${CHORES_API_PORT}"
export CHORES_WS_URL="ws://127.0.0.1:${CHORES_API_PORT}/ws"
export CHORES_API_KEY="${CHORES_API_KEY:-${CHORES_API_APIKEYS:-}}"

# SQLite path for frontend lives in /app/data
export DB_PATH="${DB_PATH:-/app/data/chores.db}"
export PORT="${PORT:-8080}"

echo "Starting Garage Trip Chores (v3.0.0)..."

# Start Go application in background
echo "Starting Go backend on ${CHORES_API_HOST}:${CHORES_API_PORT}..."
/app/main &
GO_PID=$!

# Trap signals and forward to child processes
cleanup() {
    echo "Caught termination signal. Stopping services..."
    kill -TERM "$GO_PID" "$PY_PID" 2>/dev/null || true
    wait
    exit 0
}
trap cleanup INT TERM

# Start Python FastAPI frontend in background
echo "Starting Python frontend on 0.0.0.0:${PORT}..."
/app/frontend/.venv/bin/uvicorn app.main:app --host 0.0.0.0 --port "${PORT}" --app-dir /app/frontend &
PY_PID=$!

# Wait for either process to exit
wait -n "$GO_PID" "$PY_PID"
EXIT_CODE=$?
echo "A service exited with code ${EXIT_CODE}. Shutting down container..."
kill -TERM "$GO_PID" "$PY_PID" 2>/dev/null || true
wait
exit $EXIT_CODE
