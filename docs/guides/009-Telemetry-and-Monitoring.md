# 009-Telemetry and Monitoring

## Overview

JobHunter provides a comprehensive telemetry and monitoring system with three layers:

1. **Tracking Server** (`cmd/tracker/`) -- Real-time open/click tracking via HTTP
2. **Inbox Command** (`cmd/inbox/`) -- Dashboard for engagement metrics and IMAP scan
3. **IMAP Scanner** (`internal/email/imap/`) -- Automated bounce and reply detection

---

## 1. Tracking Server

### Architecture

The tracking server is a standalone HTTP server that handles email engagement events:

```
Email HTML contains:
  <img src="https://tracker.example.com/track?id=abc-123">
  <a href="https://tracker.example.com/click?id=abc-123&url=https://...">Apply Now</a>

When recipient opens email:
  → Mail client requests /track?id=abc-123
  → Server logs open, returns 1x1 transparent GIF
  → Database: emails.opened = true, opened_at = NOW()

When recipient clicks a link:
  → Browser requests /click?id=abc-123&url=https://...
  → Server logs click, 302-redirects to original URL
  → Database: emails.clicked = true, clicked_at = NOW()
```

### Endpoints

| Endpoint | Method | Purpose | Response |
|----------|--------|---------|----------|
| `/track?id=<uuid>` | GET | Record email open | `image/gif` (43 bytes transparent GIF) |
| `/click?id=<uuid>&url=<encoded-url>` | GET | Record click + redirect | `302 Found` to original URL |
| `/health` | GET | Health check | `{"status":"ok"}` |
| `/stats` | GET | Live metrics | JSON with counters |
| `/version` | GET | Build info | JSON with Go version, uptime |
| `/` | GET | Welcome page | HTML endpoint list |

### `/stats` Response Example

```json
{
  "uptime_seconds": 86400,
  "track_hits": 142,
  "click_hits": 38,
  "track_errors": 2,
  "click_errors": 1,
  "bytes_served": 6106,
  "goroutines": 8,
  "alloc_mb": 4.2,
  "start_time": "2026-05-25T10:00:00Z"
}
```

### Server Configuration

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `TRACKING_SERVER_PORT` | `8080` | HTTP listen port |
| `TRACKING_SERVER_URL` | `http://localhost:8080` | Public URL (used in email pixels) |

### Running

```bash
# Direct
go run ./cmd/tracker/

# Docker
docker compose up -d tracker

# Custom port
TRACKING_SERVER_PORT=9000 go run ./cmd/tracker/
```

---

## 2. Inbox Command

The `inbox` command (`go run ./cmd/inbox/`) prints a comprehensive telemetry dashboard to stdout. It connects to both the database and Gmail IMAP to gather all metrics.

### Database Stats Section

Shows counts from the `emails` and `email_queue` tables:

| Metric | Source | Description |
|--------|--------|-------------|
| Total sent | `emails WHERE status='sent'` | All-time sent count |
| Pending | `emails WHERE status='pending'` | Queued but not yet sent |
| Failed | `emails WHERE status='failed'` | Send failures |
| Bounced | `emails WHERE bounced=true` | Emails that bounced |
| Replied | `emails WHERE replied=true` | Emails with human replies |
| In queue | `email_queue WHERE status='pending'` | Items awaiting send |

### Engagement Rates Section

Calculated from all-time totals:

| Rate | Calculation |
|------|-------------|
| Open rate | `opened / sent * 100` |
| Click rate | `clicked / sent * 100` |
| Reply rate | `replied / sent * 100` |
| Bounce rate | `bounced / sent * 100` |
| Deliverability | `(sent - bounced) / sent * 100` |

### Today Section

Shows today's activity counts for sent, opened, clicked, and bounced.

### IMAP Inbox Scan Section

Connects to Gmail IMAP and queries:

| Query | What it finds |
|-------|---------------|
| `FROM "mailer-daemon" SINCE <today>` | Bounce notifications received today |
| `SUBJECT "Re:" SINCE <today>` | Human replies received today |
| `UNSEEN` | Currently unread emails |
| `STATUS INBOX (MESSAGES)` | Total inbox size |

### Provider Health Section

Shows last 7 days of email performance and daily quota usage:

| Metric | Description |
|--------|-------------|
| Last 7 days -- Sent | Total sent in last 7 days |
| Bounced | Bounced in last 7 days |
| Failed | Send failures in last 7 days |
| Deliverability | (sent - bounced) / sent * 100 |
| Daily quota used | Today's sends / daily limit |
| Remaining today | Daily limit - today's sends |

### Running

```bash
# Full dashboard
go run ./cmd/inbox/

# Requires IMAP_USER and IMAP_PASS in .env for IMAP scan
# Gmail app passwords work for IMAP too
```

---

## 3. IMAP Scanner

The IMAP scanner (`internal/email/imap/scanner.go`) runs inside the inbox command to detect:

### Bounce Detection

Checks for Mailer-Daemon responses that match against sent emails by Message-ID:

| Condition | Classification | DB Update |
|-----------|---------------|-----------|
| From: `mailer-daemon` | Bounce | `bounced=true, bounce_type='hard_bounce'` |
| Subject: `Delivery Status Notification` | Delivery failure | `bounced=true, bounce_type='soft_bounce'` |
| Contains: `550` or `user unknown` | Non-existent user | `bounced=true, bounce_type='hard_bounce'` |

### Reply Detection

Checks for emails with `Subject: Re:` that reference a previously sent email:

| Condition | Action |
|-----------|--------|
| Subject starts with `Re:` + References header | Match to sent email by Message-ID |
| Subject starts with `Re:` + In-Reply-To header | Match to sent email by Message-ID |
| Match found | `replied=true, replied_at=NOW()` |

### Configuration

| Env Variable | Default | Description |
|-------------|---------|-------------|
| `IMAP_USER` | (falls back to `GMAIL_USER`) | IMAP login |
| `IMAP_PASS` | (falls back to `GMAIL_APP_PASS`) | IMAP password |
| `IMAP_HOST` | `imap.gmail.com` | IMAP server |
| `IMAP_PORT` | `993` | IMAP SSL port |

### IMAP in Workflow Commands

The IMAP scanner is **not** currently run automatically as part of the workflow pipeline. It is invoked manually via `go run ./cmd/inbox/`. Future versions may add automatic IMAP scanning to the `followup` or `send` workflows.

---

## Run Logs

All workflows record run logs to the `run_logs` table in PostgreSQL:

| Column | Description |
|--------|-------------|
| `workflow` | `scrape`, `send`, `followup`, `cleanup` |
| `status` | `completed`, `failed`, `interrupted`, `quota_exhausted` |
| `scraped` | Number of jobs scraped |
| `pending` | Jobs marked pending |
| `skipped` | Jobs skipped |
| `sent` | Emails sent |
| `failed` | Emails failed |
| `duration_ms` | Total runtime in milliseconds |
| `error_message` | Error detail if status=failed |
| `run_at` | Timestamp |

These logs can be queried directly:
```sql
SELECT * FROM run_logs ORDER BY run_at DESC LIMIT 10;
```
