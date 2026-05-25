# 001-Getting Started

## Overview

How to set up and run JobHunter for the first time.

## Prerequisites

- Go 1.26+
- A NeonDB (PostgreSQL) database -- [create free tier](https://console.neon.tech)
- A Gmail account with [App Password](https://myaccount.google.com/apppasswords) enabled
- An [OpenRouter](https://openrouter.ai/keys) API key (or Groq/Cerebras/Together/etc.)

## Step-by-Step Setup

### 1. Clone and Configure

```bash
git clone https://github.com/arinbalyan/JobHunter.git
cd JobHunter
cp .env.example .env
# Edit .env with your keys
```

### 2. Database

1. Go to [NeonDB Console](https://console.neon.tech)
2. Create a new project
3. Copy the connection string
4. Add to `.env`: `DATABASE_URL=postgres://...`

### 3. Run Migrations

Migrations run automatically on first start:

```bash
go run ./cmd/sender
```

Or manually:

```bash
go run ./cmd/migrate "$DATABASE_URL"
```

### 4. Deploy Tracking Server (Optional)

```bash
go run ./cmd/tracker
```

Starts on :8080 by default.

### 5. Run the Agent

```bash
go run ./cmd/sender
```

## What Happens on First Run

1. Migrations apply (creates all tables)
2. Plugin manager loads built-in plugins
3. JobHunter plugin scrapes job boards
4. Jobs are filtered against your profile
5. Personalized emails are sent (with tracking pixels)
6. Stats are flushed to the database
7. Summary is printed

## Next Steps

- Read [002-Plugin-System.md](002-Plugin-System.md) to understand how to add custom bots
- Read [003-Email-Tracking.md](003-Email-Tracking.md) to understand the tracking architecture
- Read [004-LLM-Router.md](004-LLM-Router.md) to understand provider routing
