# JobHunter

Automated job scraping and cold email outreach system. Scrapes job listings from
multiple boards, extracts recruiter contact information, generates personalized
cold emails, and sends them with your resume attached -- all on autopilot.

Built on [python-jobspy](https://github.com/Bunsly/JobSpy) for multi-board job
scraping.

---

## Table of Contents

- [Features](#features)
- [How It Works](#how-it-works)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Configuration](#configuration)
  - [Google Sheets Setup](#google-sheets-setup)
- [Usage](#usage)
  - [Run Modes](#run-modes)
  - [Dry Run](#dry-run)
  - [Scheduled Runs](#scheduled-runs)
- [Deployment](#deployment)
  - [GitHub Actions](#github-actions)
  - [Local Cron](#local-cron)
- [Architecture](#architecture)
  - [Project Structure](#project-structure)
  - [Two-Phase Pipeline](#two-phase-pipeline)
  - [Storage Backends](#storage-backends)
- [Job Filtering](#job-filtering)
  - [Title Filters](#title-filters)
  - [Email Filters](#email-filters)
  - [Deduplication](#deduplication)
- [Configuration Reference](#configuration-reference)
- [Development](#development)
  - [Running Tests](#running-tests)
  - [Code Style](#code-style)
- [Contributing](#contributing)
- [Credits](#credits)
- [License](#license)

---

## Features

- **Multi-board scraping** -- Indeed, LinkedIn, Glassdoor, Naukri, and more via
  python-jobspy
- **Two-phase pipeline** -- scrape and persist first, then process and email,
  with per-row status tracking for crash resilience
- **Personalized emails** -- LLM-generated cold emails via OpenRouter with
  automatic fallback to template-based generation
- **Resume attachment** -- sends your PDF resume with every outreach email
- **Google Sheets storage** -- full audit trail with separate worksheets for
  scraped jobs, sent emails, and run statistics
- **Smart deduplication** -- exact match, domain cooldown, and company cooldown
  prevent duplicate outreach
- **Title and email filtering** -- skip irrelevant job titles and generic email
  addresses automatically
- **Dry run mode** -- test the full pipeline without sending any emails
- **Configurable scheduling** -- built-in APScheduler or external cron/GitHub
  Actions
- **Weekend skip** -- automatically skips execution on weekends
- **Summary reports** -- email yourself a run summary after each execution

---

## How It Works

JobHunter runs a two-phase pipeline:

```
Phase 1: Scrape + Save
  Search terms x Locations x Boards --> python-jobspy
  --> Batch-write all results to "Scraped Jobs" worksheet (status: Pending)

Phase 2: Process + Email
  For each scraped job:
    --> Extract valid recipient emails
    --> Check deduplication rules
    --> Generate personalized email (LLM or fallback template)
    --> Send with resume attached
    --> Update row status (Yes / Skipped / Failed / DryRun)

Report:
  --> Write run statistics to "Run Stats" worksheet
  --> Email summary report to yourself
```

Every job gets a status update in the spreadsheet regardless of outcome, giving
you a complete audit trail of what was sent, what was skipped, and why.

---

## Getting Started

### Prerequisites

- Python 3.10 or higher
- [uv](https://docs.astral.sh/uv/) package manager
- A Gmail account with an
  [App Password](https://support.google.com/accounts/answer/185833)
- A Google Cloud project with the Sheets API enabled (for Google Sheets storage)
- An [OpenRouter](https://openrouter.ai/) API key (optional -- fallback
  templates work without it)

### Installation

```bash
git clone https://github.com/arinbalyan/JobHunter.git
cd JobHunter
uv sync
```

### Configuration

Copy the example environment file and fill in your values:

```bash
cp .env.example .env
```

Open `.env` in your editor and configure at minimum:

| Variable | Description |
|----------|-------------|
| `GMAIL_EMAIL` | Your Gmail address |
| `GMAIL_APP_PASSWORD` | Gmail App Password (not your regular password) |
| `RESUME_FILE_PATH` | Path to your resume PDF (default: `resume.pdf` in project root) |
| `CONTACT_NAME` | Your full name for email signatures |
| `GOOGLE_CREDENTIALS_JSON` | Base64-encoded Google service account JSON |
| `GOOGLE_SHEET_NAME` | Name of your Google Sheets spreadsheet |

See [Configuration Reference](#configuration-reference) for the full list.

### Google Sheets Setup

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the **Google Sheets API** and **Google Drive API**
4. Create a **Service Account** and download the JSON credentials file
5. Base64-encode the credentials file:
   ```bash
   base64 -w 0 credentials.json
   ```
6. Paste the output into `GOOGLE_CREDENTIALS_JSON` in your `.env`
7. Create a Google Sheets spreadsheet and share it with the service account
   email (the `client_email` field in the JSON)

JobHunter will automatically create the required worksheets (Scraped Jobs, Sent
Emails, Run Stats) on first run.

---

## Usage

### Run Modes

JobHunter supports two modes that use separate search configurations:

```bash
# Scrape onsite/hybrid jobs
uv run python -m jobspy_v2 onsite

# Scrape remote jobs
uv run python -m jobspy_v2 remote
```

Each mode uses its own set of search terms, locations, and job boards defined in
your `.env` file.

### Dry Run

Test the full pipeline without sending any emails:

```bash
DRY_RUN=true uv run python -m jobspy_v2 onsite
```

Or set `DRY_RUN=true` in your `.env` file. Dry runs still write to Google Sheets
(with status "DryRun") and track deduplication, so you can verify everything
works before going live.

### Scheduled Runs

Use the built-in scheduler to run automatically:

```bash
uv run python -m jobspy_v2 schedule
```

This starts a long-running process that triggers at your configured times. Keep
it running in a terminal, tmux session, or screen session.

Configure the schedule in your `.env`:

```
SCHEDULER_ONSITE_CRON=30 2 * * *
SCHEDULER_REMOTE_CRON=0 13 * * *
```

These are standard cron expressions (`minute hour day month weekday`). The defaults
above correspond to 8:00 AM IST (onsite) and 8:00 AM US Eastern (remote).

---

## Deployment

### GitHub Actions

The repository includes ready-to-use GitHub Actions workflows:

- `tests.yml` -- runs the test suite on every push and pull request to `main`
- `onsite.yml` -- runs the onsite scraper on a cron schedule
- `remote.yml` -- runs the remote scraper on a cron schedule

To enable the scraper workflows:

1. Push the repo to GitHub
2. Go to **Settings > Secrets and variables > Actions**
3. Add all required environment variables as repository secrets (see
   `.env.example` for the full list)
4. Upload your resume PDF as a secret or use a base64-encoded secret
5. The cron jobs will run automatically at the configured times

The test workflow runs automatically on every push and pull request to `main`.

### Local Cron

On Linux/macOS, use crontab for lightweight scheduling:

```bash
crontab -e
```

Add entries like:

```
0 9 * * 1-5  cd /path/to/JobHunter && uv run python -m jobspy_v2 onsite >> /var/log/jobhunter.log 2>&1
0 10 * * 1-5 cd /path/to/JobHunter && uv run python -m jobspy_v2 remote >> /var/log/jobhunter.log 2>&1
```

---

## Architecture

### Project Structure

```
JobHunter/
  src/jobspy_v2/
    __main__.py          # CLI entry point
    config/
      settings.py        # Pydantic settings (all from ENV)
      defaults.py        # Footer templates, reject lists
    core/
      scraper.py         # Multi-board job scraping via python-jobspy
      email_gen.py       # LLM + fallback email generation
      email_sender.py    # SMTP email sending with resume attachment
      dedup.py           # Deduplication with domain/company cooldowns
      reporter.py        # Run statistics and summary reports
    storage/
      base.py            # Storage protocol and column schemas
      sheets_backend.py  # Google Sheets implementation
      csv_backend.py     # Local CSV fallback
    workflows/
      base.py            # Two-phase pipeline orchestration
      onsite.py          # Onsite/hybrid mode
      remote.py          # Remote mode
    scheduler.py         # APScheduler-based cron scheduling
  tests/
    unit/                # 200 unit tests
  .github/workflows/     # CI and deployment workflows
  .env.example           # Configuration template
  pyproject.toml         # Project metadata and dependencies
```

### Two-Phase Pipeline

The pipeline is split into two phases for reliability:

**Phase 1 -- Scrape and Save:** All search term and location combinations are
dispatched to python-jobspy. Results are collected into a DataFrame and
batch-written to the "Scraped Jobs" worksheet with every row set to
`email_sent=Pending`. This ensures that even if the process crashes during email
sending, all scraped data is preserved.

**Phase 2 -- Process and Email:** Each scraped job is processed sequentially.
Valid recipient emails are extracted, deduplication rules are checked, a
personalized email is generated, and the email is sent with your resume. After
each job, the corresponding row in "Scraped Jobs" is updated with the outcome
(Yes, Skipped, Failed, or DryRun) along with the skip reason or recipient
address.

A 30-second interval is enforced between sends to avoid rate limiting.

### Storage Backends

| Backend | When to use | Configuration |
|---------|-------------|---------------|
| Google Sheets | Production use, full audit trail | Set `GOOGLE_CREDENTIALS_JSON` and `GOOGLE_SHEET_NAME` |
| CSV | Local testing, no Google account | Set `STORAGE_BACKEND=csv` |

Both backends implement the same protocol with three data stores:

- **Scraped Jobs** (22 columns) -- every job found during scraping
- **Sent Emails** (12 columns) -- every email successfully sent
- **Run Stats** (15 columns) -- summary statistics per run

---

## Job Filtering

### Title Filters

Jobs with titles matching any pattern in `REJECT_TITLES` are
automatically skipped. Default patterns filter out senior, lead, manager,
director, and other non-target roles. Customize in your `.env`:

```
REJECT_TITLES=senior,lead,manager,director,principal,staff,vp,chief
```

### Email Filters

Generic email addresses (info@, hr@, support@, noreply@, etc.) are filtered out
by default. Only emails that look like personal recruiter addresses are kept.
Customize with `EMAIL_FILTER_PATTERNS` in your `.env`.

### Deduplication

Three levels of deduplication prevent repeat outreach:

| Rule | Default | Description |
|------|---------|-------------|
| Exact match | Always on | Never send to the same email + company + job title combination twice |
| Domain cooldown | 5 days | Wait N days before emailing anyone at the same domain again |
| Company cooldown | 1 day | Wait N days before emailing anyone at the same company again |

Cooldown values (5 days for domain, 1 day for company) are defaults defined in
`src/jobspy_v2/core/dedup.py`. To change them, edit the constants in that file.

---

## Configuration Reference

All configuration is done through environment variables. See `.env.example` for
the complete list with descriptions and default values. Key categories:

| Category | Variables |
|----------|-----------|
| Email | `GMAIL_EMAIL`, `GMAIL_APP_PASSWORD`, `REPORT_EMAIL` |
| Identity | `CONTACT_NAME`, `CONTACT_PHONE`, `CONTACT_PORTFOLIO`, `CONTACT_GITHUB`, `CONTACT_CODOLIO` |
| Resume | `RESUME_FILE_PATH`, `RESUME_DRIVE_LINK` |
| LLM | `OPENROUTER_API_KEY`, `LLM_MODEL`, `LLM_BASE_URL` |
| Storage | `STORAGE_BACKEND`, `GOOGLE_CREDENTIALS_JSON`, `GOOGLE_SHEET_NAME` |
| Onsite Search | `ONSITE_SEARCH_TERMS`, `ONSITE_LOCATIONS`, `ONSITE_JOB_BOARDS`, `ONSITE_RESULTS_WANTED` |
| Remote Search | `REMOTE_SEARCH_TERMS`, `REMOTE_LOCATIONS`, `REMOTE_JOB_BOARDS`, `REMOTE_RESULTS_WANTED` |
| Filtering | `REJECT_TITLES`, `EMAIL_FILTER_PATTERNS` |
| Limits | `ONSITE_MAX_EMAILS_PER_DAY`, `REMOTE_MAX_EMAILS_PER_DAY`, `EMAIL_INTERVAL_SECONDS`, `MIN_EMAIL_WORDS`, `MAX_EMAIL_WORDS` |
| Schedule | `SCHEDULER_ENABLED`, `SCHEDULER_ONSITE_CRON`, `SCHEDULER_REMOTE_CRON` |
| Debug | `DRY_RUN` |

---

## Development

### Running Tests

```bash
# Run all tests
uv run pytest

# Run with coverage
uv run pytest --cov=jobspy_v2

# Run a specific test file
uv run pytest tests/unit/test_workflows.py
```

The test suite contains 200 unit tests covering configuration, storage backends,
core logic, and workflow orchestration.

### Code Style

The project uses [Ruff](https://docs.astral.sh/ruff/) for linting and
formatting:

```bash
# Check for issues
uv run ruff check src/ tests/

# Auto-fix issues
uv run ruff check --fix src/ tests/
```

Line length is set to 88 characters. Target Python version is 3.10.

---

## Contributing

Contributions are welcome. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for
guidelines on how to get started, submit issues, and open pull requests.

---

## Credits

- [python-jobspy](https://github.com/Bunsly/JobSpy) by
  [Bunsly](https://github.com/Bunsly) -- the multi-board job scraping engine
  that powers JobHunter's data collection
- [OpenRouter](https://openrouter.ai/) -- LLM API gateway used for email
  generation
- [gspread](https://github.com/burnash/gspread) -- Google Sheets API client for
  Python

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for
details.
