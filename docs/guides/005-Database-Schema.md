# 005-Database Schema

## Overview
JobHunter uses PostgreSQL (NeonDB recommended) with automatic migrations.
All migrations are embedded in the binary — no manual SQL setup needed.

## Tables

### `jobs` — Scraped job postings
```sql
id              BIGSERIAL PRIMARY KEY
job_id          TEXT NOT NULL
title           TEXT NOT NULL
company         TEXT NOT NULL
location        TEXT
is_remote       BOOLEAN
description     TEXT
url             TEXT UNIQUE          -- dedup key
source          TEXT                  -- linkedin, indeed, etc.
date_posted     TIMESTAMPTZ
fetched_at      TIMESTAMPTZ
seniority       TEXT
department      TEXT
industry        TEXT
compensation    JSONB
emails          JSONB
quality_score   INTEGER
raw_data        JSONB
```

### `emails` — Sent email tracking
```sql
id              BIGSERIAL PRIMARY KEY
job_id          BIGINT → jobs(id)
recipient_email TEXT NOT NULL
subject         TEXT
body_preview    TEXT
sent_at         TIMESTAMPTZ
status          TEXT                  -- pending | sent | failed | bounced | opened | clicked | replied
template_used   TEXT
tracking_id     TEXT UNIQUE           -- UUID for pixel tracking
message_id      TEXT                  -- SMTP Message-ID for bounce matching
opened          BOOLEAN
opened_at       TIMESTAMPTZ
clicked         BOOLEAN
clicked_at      TIMESTAMPTZ
replied         BOOLEAN
replied_at      TIMESTAMPTZ
bounced         BOOLEAN
bounced_at      TIMESTAMPTZ
bounce_type     TEXT
```

### `stats` — Time-series events
```sql
id              BIGSERIAL PRIMARY KEY
plugin_id       TEXT
event           TEXT                  -- email_sent, email_opened, plugin_run, etc.
value           DOUBLE PRECISION
tags            JSONB                 -- {"source":"linkedin","status":"success"}
recorded_at     TIMESTAMPTZ
```

### `applications` — Pipeline tracking
```sql
id              BIGSERIAL PRIMARY KEY
plugin_id       TEXT
job_id          BIGINT → jobs(id)
company         TEXT
title           TEXT
email_sent_to   TEXT
stage           TEXT                  -- queued → sent → delivered → opened → replied → interview → offer → accepted | rejected
sent_at         TIMESTAMPTZ
opened_at       TIMESTAMPTZ
replied_at      TIMESTAMPTZ
interview_at    TIMESTAMPTZ
offer_at        TIMESTAMPTZ
rejected_at     TIMESTAMPTZ
metadata        JSONB
```

### `plugin_state` — Plugin metadata
```sql
plugin_id       TEXT UNIQUE
plugin_name     TEXT
enabled         BOOLEAN
last_run_at     TIMESTAMPTZ
last_success_at TIMESTAMPTZ
run_count       INTEGER
error_count     INTEGER
config_json     JSONB
```

### `blacklist` — Bounced/rejected domains
```sql
pattern         TEXT UNIQUE           -- domain or email
reason          TEXT                  -- bounced | spam_complaint | manual
created_at      TIMESTAMPTZ
expires_at      TIMESTAMPTZ
hit_count       INTEGER
```

### `email_performance` — A/B learning
```sql
template_name   TEXT
job_seniority   TEXT
experience_match TEXT
industry        TEXT
sent_count      INTEGER
open_count      INTEGER
click_count     INTEGER
reply_count     INTEGER
bounce_count    INTEGER
UNIQUE(template_name, job_seniority, experience_match, industry)
```

## Migrations

Migrations are in `internal/migrations/` and use `golang-migrate/migrate`.
They run automatically at startup and are safe to run repeatedly.

| Migration | Description |
|-----------|-------------|
| `000001` | Core tables: jobs, emails |
| `000002` | Performance: email_performance, agent_rules, user_context |
| `000003` | Stats & plugins: stats, plugin_state, applications, blacklist |

### Running Manually
```bash
# Apply all pending migrations
go run ./cmd/migrate "$DATABASE_URL"

# Drop all tables (CAUTION: destroys data)
go run ./cmd/migrate drop "$DATABASE_URL"
```

## GitHub Actions Secrets
```bash
# Add these to your repository secrets (Settings → Secrets → Actions)
DATABASE_URL     → Your NeonDB connection string
GMAIL_USER       → Your Gmail address
GMAIL_APP_PASS   → Your Gmail App Password
OPENROUTER_API_KEY → Your OpenRouter key
```
