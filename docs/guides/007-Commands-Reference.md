# 007-Commands Reference

## Overview

JobHunter ships **11 CLI commands** under `cmd/`. Each is a standalone `main.go` that can be run with `go run ./cmd/<name>/`.

---

## 1. `scrape` -- Scrape and Filter Jobs

**Purpose:** Runs the scrappy engine, filters results against configured rules, and inserts pending/status items into the database queue.

**How it works:**
1. Loads config from `.env` and `.agent-data/config.yaml`
2. Runs migrations (if needed)
3. Locates scrappy binary -- checks `$SCRAPPY_CONFIG`, then common paths (`../scrappy/config.yaml`, `~/.scrappy/config.yaml`)
4. Executes scrappy with `--non-interactive --config` flags
5. For each job:
   - Checks **title rejection** patterns (`reject_titles`)
   - Extracts and **filters emails** (`email_filters`)
   - Runs **deduplication** (cooldown per email/domain/company)
   - Inserts as `pending` or `skipped` with reason
6. Records run log and sends Telegram report

**Reasons jobs get skipped:**
- `title_rejected` -- title matches a rejection pattern
- `no_email` -- no email address found
- `no_valid_email` -- all emails filtered out
- `email_filtered` -- email matched a filter pattern
- `dedup` -- already sent to this email/domain/company within cooldown period

**Flags:** None

**Usage:**
```bash
# Normal run
go run ./cmd/scrape/

# Set custom scrappy config path
SCRAPPY_CONFIG=/path/to/scrappy/config.yaml go run ./cmd/scrape/
```

**Output example:**
```
[info] Scrape workflow starting...
[info] using scrappy config: /home/user/projects/scrappy/config.yaml
[info] scraped 47 jobs from 5 sites
[info] results: 12 pending, 35 skipped (title_rejected=18, no_email=10, dedup=5, no_valid_email=2)
```

---

## 2. `send` -- Generate and Send Emails

**Purpose:** Picks up pending items from the email queue, generates personalized emails via LLM, and sends them via Gmail SMTP.

**How it works:**
1. Loads config and LLM provider config (`.agent-data/llm.yaml`)
2. Checks daily quota against database
3. Fetches up to `MAX_EMAILS_PER_RUN` pending queue items
4. For each item:
   - Loads user context (`.agent-data/CONTEXT.md`)
   - Builds system + user prompts for the LLM
   - Calls the LLM router (round-robin across providers)
   - Parses LLM response for subject/body
   - Injects tracking pixel (if tracking server URL configured)
   - Sends via Gmail SMTP with optional resume attachment
   - Waits `EMAIL_DELAY_SECONDS` between sends
5. Records run log and sends Telegram report

**Flags:**

| Flag | Alias | Description |
|------|-------|-------------|
| `--dry-run` | `-n` | Print would-send info without actually sending |

**Usage:**
```bash
# Dry run to see what would be sent
go run ./cmd/send/ --dry-run

# Normal run
go run ./cmd/send/

# With custom LLM provider config
go run ./cmd/send/
```

**Output example (dry-run):**
```
[info] Send workflow starting...
[info] loaded 5 active LLM providers from llm.yaml
[info] email quota: 12/500 used, 488 remaining
[info] found 8 pending emails to send
[info] [DRY-RUN] would send: subject=Interested in Software Engineer role at Acme Corp, body=1204 chars
[info] [DRY-RUN] would send: subject=Excited about Backend Engineer role at TechCo, body=985 chars
[info] send complete: 8 sent, 0 failed in 0.1s (dry-run)
```

**Output example (real run):**
```
[info] sending (1/4): Software Engineer at Acme Corp -> hr@acme.com
[info] sent successfully
[info] sending (2/4): Backend Engineer at TechCo -> careers@techco.io
[info] sent successfully
[info] send complete: 4 sent, 0 failed in 120.5s
```

---

## 3. `followup` -- Queue Follow-Up Emails

**Purpose:** Finds previously sent emails that received no reply after N days and inserts follow-up items into the queue.

**How it works:**
1. Fetches sent emails from 4+ days ago with no reply and no bounce
2. For each candidate:
   - Extracts the recipient's domain
   - Checks if a follow-up was already sent to this domain in the last 24 hours
   - If not, inserts a follow-up queue item with generated subject/body
3. Records run log and sends Telegram report

**Flags:** None

**Usage:**
```bash
go run ./cmd/followup/
```

**Output example:**
```
[info] Follow-up workflow starting...
[info] found 6 follow-up candidates
[info] queued follow-up for Software Engineer at Acme Corp (domain: acme.com)
[info] queued follow-up for Backend Engineer at TechCo (domain: techco.io)
[info] follow-up complete: 2 queued, 4 skipped in 1.2s
```

---

## 4. `cleanup` -- Remove Old Jobs

**Purpose:** Cleans up old data to keep the database lean. Deletes skipped jobs older than 14 days and marks stale pending items (7+ days) as skipped.

**How it works:**
1. Deletes all `skipped` jobs where `fetched_at` is older than 14 days
2. Finds `pending` queue items older than 7 days and marks them `skipped`
3. Records run log and sends Telegram report

**Flags:** None

**Usage:**
```bash
go run ./cmd/cleanup/
```

**Output example:**
```
[info] Cleanup workflow starting...
[info] deleted 234 skipped jobs older than 14 days
[info] marked 12 stale pending items as skipped
[info] cleanup complete in 0.8s
```

---

## 5. `tracker` -- Email Tracking Server

**Purpose:** Standalone HTTP server that serves tracking pixels and click redirects for sent emails.

**Endpoints:**

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/track?id=<uuid>` | GET | Logs an email open, returns transparent 1x1 GIF |
| `/click?id=<uuid>&url=<encoded>` | GET | Logs a click, 302-redirects to original URL |
| `/health` | GET | Returns `{"status":"ok"}` |
| `/stats` | GET | Live JSON metrics (hits, errors, uptime) |
| `/version` | GET | Build and runtime information |
| `/` | GET | Welcome page with endpoint list |

**Usage:**
```bash
# Default port :8080
go run ./cmd/tracker/

# Custom port
TRACKING_SERVER_PORT=9000 go run ./cmd/tracker/
```

**Output example:**
```
starting tracking server on :8080
```

**Docker:**
```bash
# Run via Docker Compose
docker compose up -d tracker
```

---

## 6. `inbox` -- Telemetry Dashboard

**Purpose:** Displays comprehensive email campaign telemetry: database stats, engagement rates, IMAP inbox scan, and provider health.

**Output sections:**
- **Database Stats** -- total sent, pending, failed, bounced, replied; queue size
- **Engagement Rates** -- open rate, click rate, reply rate, bounce rate, deliverability
- **Today** -- today's sent, opened, clicked, bounced counts
- **IMAP Inbox Scan** -- connects to Gmail IMAP, shows bounces today, replies today, unread count, inbox total
- **Provider Health** -- last 7 days sent/bounced/failed, daily quota usage

**Flags:** None

**Usage:**
```bash
go run ./cmd/inbox/
```

**Output example:**
```
  JobHunter Inbox Telemetry
  ──────────────────────────────────────────────────

  📊 Database Stats

  Total sent:           142
  Pending:              8
  Failed:               3
  Bounced:              5
  Replied:              2
  In queue:             8

  📈 Engagement Rates

  Opened:               38/142 (26.8%)
  Clicked:              12/142 (8.5%)
  Replied:              2/142 (1.4%)
  Bounced:              5/142 (3.5%)
  Deliverability:       96.5%

  📅 Today

  Sent:                 4
  Opened:               1
  Clicked:              0
  Bounced:              0

  📬 IMAP Inbox Scan

  ✓ Connected to Gmail IMAP
  ✓ Bounces today: 1
  ✓ Replies today: 0
  ✓ Unread emails: 42
  ✓ Inbox total: 867

  📋 Provider Health

  Last 7 days — Sent:        28
  Bounced:                   2
  Failed:                    0
  Deliverability:            92.9%
  Daily quota used:          4/500
  Remaining today:           496
```

---

## 7. `doctor` -- Diagnostic Tool

**Purpose:** 15-point checklist that verifies all dependencies and configuration are correct. Auto-creates `.env` from `.env.example` if missing.

**Checks performed:**

| # | Check | What it verifies |
|---|-------|------------------|
| 1 | `.env` file | Exists or auto-create from `.env.example` |
| 2 | Config loading | Can load and parse env vars |
| 3 | Config validation | Required vars are present |
| 4 | `config.yaml` | Loads `.agent-data/config.yaml` |
| 5 | `llm.yaml` | Loads `.agent-data/llm.yaml` |
| 6 | Database connection | TCP reachable to Postgres host |
| 7 | Gmail SMTP | TCP reachable to `smtp.gmail.com:587` |
| 8 | OpenRouter API | API authentication, model count |
| 9 | Other LLM providers | Count of configured additional providers |
| 10 | scrappy binary | In PATH or not |
| 11 | Telegram bot | Token + chat ID configured |
| 12 | IMAP | TCP reachable to `imap.gmail.com:993` |
| 13 | GitHub CLI (`gh`) | In PATH or not |
| 14 | Resume PDF | Found in `.agent-data/` directory |
| 15 | DB migrations | Reminder to run `go run ./cmd/migrate` |

**Usage:**
```bash
go run ./cmd/doctor/
```

**Output example:**
```
  JobHunter Doctor
  ──────────────────────────────────────────────────

  [1] .env file .............. ✓ found
  [2] Config loading ......... ✓
  [3] Config validation ...... ✓
  [4] config.yaml ............ ✓ 47 rejection patterns, 18 email filters
  [5] llm.yaml ............... ✓
  [6] Database connection .... ✓ reachable
  [7] Gmail SMTP ............ ✓ reachable
  [8] OpenRouter API ........ ✓ connected (28 models available)
  [9] Other LLM providers .... ✓ 3 additional providers configured
  [10] scrappy binary ......... ⚠ not in PATH
  [11] Telegram bot ........... ✓ configured
  [12] IMAP (bounce/reply) .... ✓ reachable
  [13] GitHub CLI (gh) ........ ✓ found
  [14] Resume PDF ............. ⚠ not found in .agent-data/
  [15] DB migrations .......... ⚠ run 'go run ./cmd/migrate' to verify

  ──────────────────────────────────────────────────
  All checks passed!
```

---

## 8. `migrate` -- Database Migrations

**Purpose:** Applies or drops database migrations.

**Flags:**

| Argument | Description |
|----------|-------------|
| `<database_url>` | Apply all pending migrations |
| `drop <database_url>` | **CAUTION:** Drop all tables |

**Usage:**
```bash
# Apply all pending migrations
go run ./cmd/migrate "postgres://user:pass@host:5432/db"

# Drop all tables (data loss!)
go run ./cmd/migrate drop "postgres://user:pass@host:5432/db"
```

**Output example:**
```
Running migrations...
Migrations complete.
Current schema version: 5
```

---

## 9. `botid` -- Discover Telegram Chat ID

**Purpose:** One-time utility to discover your Telegram chat ID for bot notifications.

**How it works:**
1. Reads `TELEGRAM_BOT_TOKEN` from env
2. Polls Telegram API for updates (waits up to 120 seconds)
3. Asks you to message your bot on Telegram
4. Prints your chat ID once a message is received

**Usage:**
```bash
TELEGRAM_BOT_TOKEN=your_bot_token go run ./cmd/botid/
```

**Output example:**
```
Waiting for a message from you...
Open Telegram, find your bot, and send it any message (even just 'hi').

═══════════════════════════════════════════
  Your Telegram Chat ID: 123456789
═══════════════════════════════════════════

Add this to your .env:
  TELEGRAM_CHAT_ID=123456789
```

---

## 10. `syncsecrets` -- Push Secrets to GitHub

**Purpose:** Reads `.env` and pushes all values to GitHub repository secrets via the `gh` CLI.

**How it works:**
1. Verifies `gh` CLI is installed and authenticated
2. Reads `.env` file
3. Shows a preview of found variables
4. Prompts for confirmation
5. Pushes each variable as a GitHub secret
6. Skips system vars (PATH, HOME, etc.) and empty vars

**Prerequisites:**
- [GitHub CLI](https://cli.github.com/) installed
- `gh auth login` completed
- Write access to the repository

**Usage:**
```bash
go run ./cmd/syncsecrets/
```

**Output example:**
```
  JobHunter Secret Sync
  ──────────────────────────────────────────────────

  Found 18 variables in .env

  This will push ALL values to GitHub Secrets.
  Continue? [y/N]: y

  ✓ DATABASE_URL
  ✓ GMAIL_USER
  ✓ GMAIL_APP_PASS
  ✓ OPENROUTER_API_KEY
  ✓ TELEGRAM_BOT_TOKEN

  ──────────────────────────────────────────────────
  Synced: 5 | Skipped: 13 | Errors: 0
  All secrets synced successfully!
```
