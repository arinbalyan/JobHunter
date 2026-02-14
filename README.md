# JobSpy V2

Automated job scraping and cold email outreach system. Scrapes job listings from multiple boards, generates personalized cold emails via LLM (with smart fallback), deduplicates contacts, and sends outreach — all driven by environment variables.

## Features

- **Multi-board scraping** — Indeed, LinkedIn, Glassdoor, Google, Naukri, ZipRecruiter, Bayt, BDJobs via [python-jobspy](https://github.com/Bunsly/JobSpy)
- **Parallel scraping** — ThreadPoolExecutor dispatches location × search term × board concurrently
- **LLM-powered emails** — OpenRouter-based generation with automatic fallback to templates
- **Smart deduplication** — Exact email, domain cooldown (5 days), company same-day checks
- **Google Sheets persistence** — Data survives across CI/CD runs (CSV fallback included)
- **Two workflows** — `onsite` (India, IST 8 AM) and `remote` (global, US ET 8 AM)
- **Zero hardcoded values** — Everything configurable via `.env`
- **Platform-agnostic deployment** — Render, Railway, GitHub Actions, or local CLI

## Quick Start

### Prerequisites

- Python 3.10+
- [uv](https://docs.astral.sh/uv/) (recommended) or pip

### Installation

```bash
# Clone the repo
git clone https://github.com/yourusername/jobSpy-V2.git
cd jobSpy-V2

# Install dependencies
uv sync
```

### Configuration

```bash
# Copy the example config
cp .env.example .env

# Edit .env with your values
# See .env.example for detailed setup instructions for each section
```

**Required secrets:**
| Variable | Description |
|---|---|
| `GMAIL_EMAIL` | Gmail address for sending emails |
| `GMAIL_APP_PASSWORD` | [Gmail App Password](https://myaccount.google.com/apppasswords) |
| `OPENROUTER_API_KEY` | [OpenRouter API key](https://openrouter.ai/keys) for LLM |
| `CONTACT_NAME` | Your full name (used in emails) |

**Optional but recommended:**
| Variable | Description |
|---|---|
| `GOOGLE_CREDENTIALS_JSON` | Base64-encoded service account JSON for Google Sheets |
| `REPORT_EMAIL` | Email address for end-of-run reports |
| `RESUME_FILE_PATH` | Path to resume PDF (attached to emails) |

See `.env.example` for the complete list of 40+ configurable variables.

### Usage

```bash
# Run onsite workflow (scrape India jobs + send emails)
uv run python -m jobspy_v2 onsite

# Run remote workflow (scrape remote jobs + send emails)
uv run python -m jobspy_v2 remote

# Dry run — scrape and generate emails, but don't send
uv run python -m jobspy_v2 onsite --dry-run
uv run python -m jobspy_v2 remote --dry-run

# Serve mode — long-running scheduler for PaaS (Render/Railway)
uv run python -m jobspy_v2 serve
```

## Architecture

```
src/jobspy_v2/
├── __init__.py            # Package init, version
├── __main__.py            # CLI: onsite | remote | serve [--dry-run]
├── config/
│   ├── settings.py        # Pydantic BaseSettings (all ENV loading)
│   └── defaults.py        # Default reject titles, filter patterns, prompt templates
├── core/
│   ├── scraper.py         # JobSpy wrapper, board-specific param adaptation, parallel scraping
│   ├── email_gen.py       # LLM email generation with fallback
│   ├── email_sender.py    # SMTP with 3-retry, PDF attachment
│   ├── dedup.py           # Dedup: exact email, domain cooldown, company same-day
│   └── reporter.py        # End-of-run summary email + stats recording
├── storage/
│   ├── base.py            # StorageBackend Protocol
│   ├── csv_backend.py     # CSV fallback (3 files)
│   └── sheets_backend.py  # Google Sheets via gspread
├── scheduler/
│   ├── cron_scheduler.py  # APScheduler v3 BackgroundScheduler wrapper
│   ├── health_check.py    # HTTP health endpoint for PaaS keep-alive
│   └── runner.py          # Serve-mode orchestrator with signal handling
├── workflows/
│   ├── base.py            # BaseWorkflow pipeline
│   ├── onsite.py          # OnsiteWorkflow (mode="onsite")
│   └── remote.py          # RemoteWorkflow (mode="remote")
└── utils/
    ├── email_utils.py     # Email regex, validation, filtering
    └── text_utils.py      # HTML/MD stripping, word count, truncation
```

### Pipeline Flow

```
1. Skip weekends (configurable)
2. Load applicant context (contexts/profile.md)
3. Scrape jobs (parallel: locations × terms × boards)
4. For each job with valid emails:
   a. Filter spam/no-reply emails
   b. Check dedup (exact + domain cooldown + company same-day)
   c. Generate personalized email (LLM → fallback)
   d. Send via SMTP (or log in dry-run mode)
   e. Mark as sent in storage
   f. Sleep between sends (configurable, default 30s)
5. Send end-of-run report email
```

## Deployment

### Option 1: GitHub Actions (Recommended for free tier)

The repo includes three workflow files:

| Workflow | File | Schedule | Description |
|---|---|---|---|
| Tests | `.github/workflows/tests.yml` | On push/PR | Lint + test on Python 3.10–3.12 |
| Onsite | `.github/workflows/onsite.yml` | Daily 2:30 UTC (IST 8 AM) | Run onsite job workflow |
| Remote | `.github/workflows/remote.yml` | Daily 13:00 UTC (US ET 8 AM) | Run remote job workflow |

**Setup:**
1. Push to GitHub
2. Go to **Settings → Secrets and variables → Actions**
3. Add all required secrets from `.env.example`
4. Workflows run automatically on schedule (or trigger manually via **Actions → Run workflow**)

### Option 2: Render (Background Worker)

```bash
# Deploy via Render Blueprint
# 1. Push to GitHub
# 2. Render Dashboard → New → Blueprint → Connect repo
# 3. Render reads render.yaml and creates the service
# 4. Add environment variables in Render dashboard
```

The included `render.yaml` configures a free-tier background worker with Docker.

### Option 3: Docker (Any Platform)

```bash
# Build
docker build -t jobspy-v2 .

# Run with env file
docker run --env-file .env jobspy-v2

# Run specific workflow
docker run --env-file .env jobspy-v2 python -m jobspy_v2 onsite --dry-run
```

### Option 4: Local Cron

```bash
# Add to crontab (example: IST 8 AM daily)
30 2 * * * cd /path/to/jobSpy-V2 && uv run python -m jobspy_v2 onsite >> /var/log/jobspy-onsite.log 2>&1
```

## Storage Backends

### Google Sheets (Default)

Persists data across CI/CD runs and devices. See `.env.example` for the 6-step setup guide.

Three worksheets are auto-created:
- **Sent Emails** — All sent email records
- **Scraped Jobs** — Raw job listing data
- **Run Stats** — Per-run statistics

### CSV Fallback

When Google Sheets credentials are missing or invalid, the system automatically falls back to local CSV files. Set `STORAGE_BACKEND=csv` to use CSV explicitly.

## Job Filtering

### Title Rejection

37 built-in patterns filter out irrelevant job titles (teacher, nurse, cashier, driver, chef, etc.). Override via `REJECT_TITLES` env var.

### Email Filtering

6 built-in patterns filter spam/no-reply emails:
```
starts_with:accommodation@, contains:accessibility,
contains:no-reply, contains:noreply, contains:do-not-reply
```
Override via `EMAIL_FILTER_PATTERNS` env var.

### Deduplication Rules

1. **Exact email** — Never email the same address twice
2. **Domain cooldown** — 5-day cooldown per domain
3. **Company same-day** — Max 1 email per company per day

## Development

```bash
# Install all dependencies (including dev)
uv sync

# Run tests
uv run pytest --tb=short -q

# Run tests with coverage
uv run pytest --cov=jobspy_v2 --cov-report=term-missing

# Lint
uv run ruff check src/ tests/

# Format
uv run ruff format src/ tests/
```

### Test Suite

197 tests covering:
- **Config** (33) — Settings loading, CSV parsing, defaults, validation
- **Utils** (70) — Email validation, extraction, filtering, text processing
- **Storage** (30) — CSV backend, Sheets backend, factory pattern
- **Core** (27) — Scraper params, dedup rules, email generation, sender, reporter
- **Scheduler** (18) — Cron parsing, scheduler lifecycle, health check, runner
- **Workflows** (19) — Pipeline flow, weekend skipping, dry run, CLI

## Environment Variables Reference

All variables are documented in `.env.example`. Key categories:

| Category | Prefix | Example |
|---|---|---|
| SMTP | `GMAIL_*`, `SMTP_*` | `GMAIL_EMAIL`, `SMTP_PORT` |
| LLM | `OPENROUTER_*`, `LLM_*` | `OPENROUTER_API_KEY`, `LLM_MODEL` |
| Contact | `CONTACT_*` | `CONTACT_NAME`, `CONTACT_PHONE` |
| Storage | `GOOGLE_*`, `CSV_*` | `GOOGLE_SHEET_NAME`, `CSV_FILE_PATH` |
| Scheduler | `SCHEDULER_*`, `HEALTH_CHECK_*` | `SCHEDULER_ONSITE_CRON` |
| Onsite | `ONSITE_*` | `ONSITE_SEARCH_TERMS`, `ONSITE_LOCATIONS` |
| Remote | `REMOTE_*` | `REMOTE_SEARCH_TERMS`, `REMOTE_LOCATION` |
| Email | `MIN_EMAIL_WORDS`, `MAX_EMAIL_WORDS` | `EMAIL_INTERVAL_SECONDS` |
| Filter | `REJECT_TITLES`, `EMAIL_FILTER_PATTERNS` | Comma-separated patterns |

## License

MIT
