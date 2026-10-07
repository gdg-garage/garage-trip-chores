# Garage Trip Chores — production image
FROM ghcr.io/astral-sh/uv:python3.12-bookworm-slim

WORKDIR /app

# Install dependencies first (better layer caching).
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev

# Application code.
COPY app ./app

# Use the venv uv created.
ENV PATH="/app/.venv/bin:$PATH"
# SQLite lives on the mounted Fly volume so data survives restarts/deploys.
ENV DB_PATH=/data/chores.db
ENV PORT=8080

EXPOSE 8080
CMD ["sh", "-c", "uvicorn app.main:app --host 0.0.0.0 --port ${PORT:-8080}"]
