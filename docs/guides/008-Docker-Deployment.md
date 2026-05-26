# 008-Docker Deployment

## Overview

JobHunter includes Docker support for local development. The `docker-compose.yml` spins up:

- **PostgreSQL 16** (NeonDB-compatible local database)
- **Tracking server** (compiled Dockerfile.tracker)

> **Note:** The main workflow commands (`scrape`, `send`, `followup`, `cleanup`) run as ephemeral Go processes and are typically run outside Docker. Only the tracker runs as a long-lived service in the container setup.

## Prerequisites

- Docker and Docker Compose v2
- Go 1.26+ (for running commands locally)

## Quick Start

```bash
# Start all services
docker compose up -d

# Verify everything is running
docker compose ps

# Check logs
docker compose logs -f
```

## Services

### PostgreSQL (`postgres`)

- **Image:** `postgres:16-alpine`
- **Port:** `5432`
- **User:** `jobhunter`
- **Password:** `jobhunter_dev`
- **Database:** `jobhunter`
- **Volume:** `pgdata` (persists data across restarts)
- **Health check:** pg_isready every 5 seconds

Connection string for local development:
```
DATABASE_URL=postgres://jobhunter:jobhunter_dev@localhost:5432/jobhunter?sslmode=disable
```

### Tracking Server (`tracker`)

- **Build:** Multi-stage Dockerfile (`Dockerfile.tracker`)
- **Port:** `8080`
- **Depends on:** PostgreSQL (waits for healthy)
- **Restart:** Unless stopped

The tracker is compiled into a slim Alpine image via multi-stage build:
1. **Builder stage:** Full `golang:1.26-alpine` -- compiles `cmd/tracker/`
2. **Runtime stage:** Minimal `alpine:3.20` with `ca-certificates` and the compiled binary

## Configuration

### Environment Variables

Set these in `docker-compose.yml` under the `tracker` service's `environment`:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | (see compose file) | Postgres connection string |
| `TRACKING_SERVER_PORT` | `8080` | HTTP listen port |
| `GOMEMLIMIT` | `50MiB` | Go garbage collector memory limit |

### Port Mapping

The tracker exposes port `8080`. To use a different host port:

```yaml
ports:
  - "9090:8080"   # Host:9090 -> Container:8080
```

## Building Images

### Tracking Server Only

```bash
# Build the tracking server image
docker build -t jobhunter-tracker -f Dockerfile.tracker .

# Run it manually
docker run -p 8080:8080 \
  -e DATABASE_URL=postgres://jobhunter:jobhunter_dev@host.docker.internal:5432/jobhunter?sslmode=disable \
  -e TRACKING_SERVER_PORT=8080 \
  jobhunter-tracker
```

### Full Development Image

The `Dockerfile` provides a full Go development environment:

```bash
# Build development image
docker build -t jobhunter-dev .

# Run doctor inside container
docker run --rm -v $(pwd)/.env:/app/.env jobhunter-dev go run ./cmd/doctor/
```

## Local Development Workflow

```bash
# Terminal 1: Start Docker services
docker compose up -d

# Terminal 2: Run doctor
go run ./cmd/doctor/

# Set your .env DATABASE_URL to local:
# DATABASE_URL=postgres://jobhunter:jobhunter_dev@localhost:5432/jobhunter?sslmode=disable

# Run migrations
go run ./cmd/migrate "postgres://jobhunter:jobhunter_dev@localhost:5432/jobhunter?sslmode=disable"

# Scrape jobs
go run ./cmd/scrape/

# Check telemetry
go run ./cmd/inbox/
```

## Stopping

```bash
# Stop services (keeps data volume)
docker compose stop

# Stop and remove containers
docker compose down

# Stop and remove containers + data volume
docker compose down -v
```

## Deploying to Production

For production deployment, the tracking server can be deployed to:

- **Vercel** (via `vercel.json` and `.github/workflows/deploy-tracker.yml`)
- **Railway** or **Render** as a web service
- **Any Docker host** with the built image

### Vercel Deployment

The `vercel.json` at the project root configures Vercel to build `cmd/tracker/main.go` as a Go serverless function:

```json
{
  "builds": [
    {
      "src": "cmd/tracker/main.go",
      "use": "@vercel/go"
    }
  ],
  "routes": [
    { "src": "/track(.*)", "dest": "cmd/tracker/main.go" },
    { "src": "/click(.*)", "dest": "cmd/tracker/main.go" },
    { "src": "/health", "dest": "cmd/tracker/main.go" }
  ]
}
```

The GitHub Action `.github/workflows/deploy-tracker.yml` automates deployment:

```bash
# Or deploy manually
vercel deploy --prod \
  --build-env DATABASE_URL=$DATABASE_URL \
  --build-env TRACKING_SERVER_PORT=8080 \
  --env DATABASE_URL=$DATABASE_URL \
  --confirm
```

After deployment, set `TRACKING_SERVER_URL` in your `.env`:
```
TRACKING_SERVER_URL=https://your-project.vercel.app
```
