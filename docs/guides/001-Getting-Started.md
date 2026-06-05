# 001-Getting Started

## Overview

How to set up and run JobHunter for the first time.

## Prerequisites

- Go 1.26+
- A NeonDB (PostgreSQL) database -- [create free tier](https://console.neon.tech)
- A Gmail account with [App Password](https://myaccount.google.com/apppasswords) enabled
- An [OpenRouter](https://openrouter.ai/keys) API key (or any of 10+ supported LLM providers)
- Docker (optional, for local Postgres)

## Step-by-Step Setup

### 1. Clone and Configure

```bash
git clone https://github.com/arinbalyan/JobHunter.git
cd JobHunter
```

### 2. Run Doctor (Automated Setup)

```bash
go run ./cmd/doctor/
```

The doctor:
- **Auto-creates** `.env` from `.env.example` if it doesn't exist
- Checks all 15 configuration points (database, SMTP, LLM providers, Telegram, etc.)
- Shows green checkmarks for passing checks and red crosses for failures

Then edit `.env` with your real API keys.

### 3. Database

**Option A: Local Docker (for development)**
```bash
docker compose up -d postgres
# Connection string:
# DATABASE_URL=postgres://jobhunter:jobhunter_dev@localhost:5432/jobhunter?sslmode=disable
```

**Option B: NeonDB (for production/GitHub Actions)**
1. Go to [NeonDB Console](https://console.neon.tech)
2. Create a new project
3. Copy the connection string
4. Add to `.env`: `DATABASE_URL=postgres://...`

### 4. Run Migrations

Migrations run automatically on first start of any command. To run manually:

```bash
go run ./cmd/migrate "$DATABASE_URL"
```

### 5. Scrape Jobs

```bash
go run ./cmd/scrape/
```

This runs the scrappy engine, filters results, and inserts pending items into the email queue.

### 6. Send Emails (Dry Run First)

```bash
# See what would be sent without actually sending
go run ./cmd/send/ --dry-run

# Then send for real
go run ./cmd/send/
```

### 7. Deploy Tracking Server (Optional, for Open/Click Tracking)

```bash
# Run locally
go run ./cmd/tracker/

# Or via Docker
docker compose up -d tracker
```

Starts on :8080 by default. Emails will include tracking pixels pointing to this URL.

### 8. View Telemetry

```bash
go run ./cmd/inbox/
```

Shows engagement rates, today's stats, IMAP scan, and provider health.

## Workflow Pipeline (Typical Day)

```
1. docker compose up -d                  # Start Postgres + tracker
2. go run ./cmd/scrape/                   # 6 AM: Scrape job boards
3. go run ./cmd/send/                     # 8 AM: Send pending emails
4. go run ./cmd/followup/                 # 9 AM: Queue follow-ups
5. go run ./cmd/inbox/                    # Check telemetry
6. go run ./cmd/cleanup/                  # Weekly: Clean old data
```

In production, steps 2-6 are automated via GitHub Actions (see [006-GitHub-Actions](006-GitHub-Actions.md)).

## What Happens on Each Run

- **scrape**: scrappy builds from source (or uses local binary) -> scrapes 55+ boards -> filters by title, email, dedup -> inserts pending/skipped jobs -> Telegram report
- **send**: Fetches pending queue -> generates LLM email for each -> injects tracking pixel -> sends via Gmail SMTP with 30s delay -> Telegram report
- **followup**: Finds sent+no-reply emails from 4+ days ago -> queues follow-up items -> Telegram report
- **cleanup**: Deletes skipped jobs older than 14 days -> marks stale pending as skipped -> Telegram report
- **inbox**: Queries database stats -> connects to Gmail IMAP -> prints engagement dashboard

## Next Steps

- Read [007-Commands-Reference.md](007-Commands-Reference.md) for detailed docs on every CLI command
- Read [002-Plugin-System.md](002-Plugin-System.md) to understand how to add custom bots
- Read [003-Email-Tracking.md](003-Email-Tracking.md) to understand the tracking architecture
- Read [004-LLM-Router.md](004-LLM-Router.md) to understand provider routing
- Read [008-Docker-Deployment.md](008-Docker-Deployment.md) for Docker setup details
