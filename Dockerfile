# Stage 1: Build the Go application
FROM golang:1.25-bookworm AS go-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    libc6-dev \
    libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o /app/main .

# Stage 2: Install Python frontend dependencies with uv
FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim AS py-builder

WORKDIR /app/frontend

COPY frontend/pyproject.toml frontend/uv.lock ./
RUN uv sync --frozen --no-dev

# Stage 3: Unified production image
FROM python:3.12-slim-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    sqlite3 \
    procps \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy Go binary and documentation
COPY --from=go-builder /app/main /app/main
COPY --from=go-builder /app/docs /app/docs

# Copy Python virtual environment and frontend application
COPY --from=py-builder /app/frontend/.venv /app/frontend/.venv
COPY frontend /app/frontend

# Copy entrypoint script
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Persistent directory for SQLite databases
RUN mkdir -p /app/data

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/entrypoint.sh"]
