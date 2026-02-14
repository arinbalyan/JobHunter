# ---------------------------------------------------------------------------
# JobSpy V2 — Multi-stage Docker build
# Runs in "serve" mode by default (APScheduler cron + HTTP health check)
# ---------------------------------------------------------------------------
FROM python:3.12-slim AS base

# Prevent Python from writing .pyc files and enable unbuffered output
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

# ---------------------------------------------------------------------------
# Stage 1: Install uv + dependencies
# ---------------------------------------------------------------------------
FROM base AS deps

# Install uv (fast Python package manager)
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# Copy only dependency files first (for layer caching)
COPY pyproject.toml uv.lock* ./

# Install production dependencies only (no dev deps)
RUN uv sync --no-dev --frozen 2>/dev/null || uv sync --no-dev

# ---------------------------------------------------------------------------
# Stage 2: Final runtime image
# ---------------------------------------------------------------------------
FROM base AS runtime

# Copy installed virtualenv from deps stage
COPY --from=deps /app/.venv /app/.venv

# Put virtualenv on PATH
ENV PATH="/app/.venv/bin:$PATH"

# Copy application source
COPY src/ ./src/
COPY contexts/ ./contexts/

# Expose health check port (configurable via HEALTH_CHECK_PORT env)
EXPOSE 10000

# Default: serve mode (scheduler + health check)
# Override with: docker run ... python -m jobspy_v2 onsite --dry-run
CMD ["python", "-m", "jobspy_v2", "serve"]
