# JobHunter

Automated job scraping and cold email outreach system. Scrapes job listings from
multiple boards, extracts recruiter contact information, generates personalized
cold emails, and sends them with your resume attached -- all on autopilot.

Built on [python-jobspy](https://github.com/SpeedyApply/JobSpy) for multi-board
job scraping.

---

## Table of Contents

- [Features](#features)
- [How It Works](#how-it-works)
- [Quick Start (Zero to First Run)](#quick-start-zero-to-first-run)
  - [Step 1: Clone and Install](#step-1-clone-and-install)
  - [Step 2: Create Your Applicant Profile](#step-2-create-your-applicant-profile)
  - [Step 3: Set Up Google Sheets](#step-3-set-up-google-sheets)
  - [Step 4: Configure Environment](#step-4-configure-environment)
  - [Step 5: Add Your Resume](#step-5-add-your-resume)
  - [Step 6: Test With a Dry Run](#step-6-test-with-a-dry-run)
  - [Step 7: Go Live](#step-7-go-live)
- [Usage](#usage)
  - [Run Modes](#run-modes)
  - [Dry Run](#dry-run)
  - [Scheduled Runs](#scheduled-runs)
- [Deployment](#deployment)
  - [GitHub Actions (Full Setup)](#github-actions-full-setup)
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

- **Multi-board scraping** -- Indeed, LinkedIn, Glassdoor, and more via
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

## Quick Start (Zero to First Run)

This section walks you through everything from a fresh clone to your first
successful dry run. Follow these steps in order.

### Step 1: Clone and Install

You need Python 3.10+ and [uv](https://docs.astral.sh/uv/) (a fast Python
package manager).

```bash
# Install uv if you don't have it
curl -LsSf https://astral.sh/uv/install.sh | sh

# Clone the repo
git clone https://github.com/arinbalyan/JobHunter.git
cd JobHunter

# Install all dependencies
uv sync
```

After `uv sync`, you should see a `.venv` directory in the project root. All
commands use `uv run` to automatically pick up this virtual environment.

### Step 2: Create Your Applicant Profile

JobHunter uses an **applicant profile** to generate personalized cold emails.
This is a plain text or markdown file that describes who you are, what you have
done, and what you are looking for. The LLM reads this file and uses it as
context when writing emails on your behalf.

Create the file at `contexts/profile.md`:

```bash
mkdir -p contexts
```

Then create `contexts/profile.md` with your information. Here is an example
structure:

```markdown
# Your Name

## Summary
Recent CS graduate / 3 years of experience in backend engineering / looking
for roles in machine learning, data science, or software engineering.

## Skills
- Languages: Python, JavaScript, SQL, Go
- ML/AI: PyTorch, scikit-learn, pandas, NLP, computer vision
- Web: FastAPI, React, Node.js, PostgreSQL
- Cloud: AWS (EC2, S3, Lambda), Docker, Kubernetes

## Experience
- Software Engineer at CompanyX (2022-2024): Built data pipelines processing
  10M+ records daily. Deployed ML models for recommendation engine.
- Intern at StartupY (Summer 2021): Developed REST API for internal tools.

## Education
- B.Tech in Computer Science, University Name, 2024
- Relevant coursework: Machine Learning, Data Structures, Distributed Systems

## Projects
- Built a job search automation tool using Python and LLMs
- Open-source contributor to [project name]
- Personal portfolio: yoursite.com

## What I'm Looking For
Entry-level to mid-level roles in ML engineering, data science, or full-stack
development. Open to onsite (India) and remote positions.
```

Write it in **first person or third person**, either works. The LLM will adapt.
The more specific you are about your skills and experience, the better the
generated emails will be.

This file is gitignored and never committed. For GitHub Actions deployment, you
will base64-encode it as a secret (covered in the [Deployment](#deployment)
section).

### Step 3: Set Up Google Sheets

JobHunter stores everything in Google Sheets -- scraped jobs, sent emails, and
run statistics. Here is how to set it up:

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (or select an existing one)
3. In the left sidebar, go to **APIs & Services > Library**
4. Search for and enable both:
   - **Google Sheets API**
   - **Google Drive API**
5. Go to **APIs & Services > Credentials**
6. Click **Create Credentials > Service Account**
7. Give it a name (e.g., "jobhunter-bot") and click through to finish
8. Click on the new service account, go to the **Keys** tab
9. Click **Add Key > Create New Key > JSON** and download it
10. Base64-encode the downloaded JSON file:
    ```bash
    # Linux/macOS/WSL
    base64 -w 0 your-credentials-file.json
    # Copy the entire output (it will be a long string)
    ```
11. Go to [Google Sheets](https://sheets.google.com/) and create a new
    spreadsheet. Name it whatever you want (e.g., "JobHunter")
12. Share the spreadsheet with the service account email. You can find this
    email in the JSON file under `client_email` (it looks like
    `jobhunter-bot@your-project.iam.gserviceaccount.com`). Give it **Editor**
    access.

JobHunter will automatically create the required worksheets (Scraped Jobs, Sent
Emails, Run Stats) on the first run.

### Step 4: Configure Environment

Copy the example file and fill in your values:

```bash
cp .env.example .env
```

Open `.env` in your editor. Here is what to fill in, grouped by priority:

**Must set (will not work without these):**

| Variable | What to Put |
|----------|-------------|
| `GMAIL_EMAIL` | Your Gmail address |
| `GMAIL_APP_PASSWORD` | A Gmail App Password ([generate here](https://myaccount.google.com/apppasswords)) -- this is NOT your Gmail password |
| `CONTACT_NAME` | Your full name |
| `GOOGLE_CREDENTIALS_JSON` | The base64 string from Step 3 |
| `GOOGLE_SHEET_NAME` | The name of the spreadsheet from Step 3 |

**Should set (for better email quality):**

| Variable | What to Put |
|----------|-------------|
| `OPENROUTER_API_KEY` | Get a free key at [openrouter.ai/keys](https://openrouter.ai/keys) -- without this, emails use basic templates instead of LLM |
| `CONTACT_EMAIL` | Your contact email for email signatures |
| `CONTACT_PHONE` | Your phone number |
| `CONTACT_PORTFOLIO` | Your portfolio URL |
| `CONTACT_GITHUB` | Your GitHub username |
| `RESUME_DRIVE_LINK` | Google Drive link to your resume |

**Optional (defaults work fine):**

| Variable | Default | What It Does |
|----------|---------|--------------|
| `CONTEXT_FILE_PATH` | `contexts/profile.md` | Path to your applicant profile from Step 2 |
| `RESUME_FILE_PATH` | `resume.pdf` | Path to your resume PDF (see Step 5) |
| `REPORT_EMAIL` | Same as `GMAIL_EMAIL` | Where to send run summary reports |
| `STORAGE_BACKEND` | `sheets` | Use `csv` for local testing without Google Sheets |
| `DRY_RUN` | `false` | Set to `true` to test without sending emails |
| `ONSITE_RESULTS_WANTED` | `1000` | Max jobs per search term per board (200-500 recommended) |

**Customize your job search:**

| Variable | Example | Description |
|----------|---------|-------------|
| `ONSITE_SEARCH_TERMS` | `machine learning,data scientist,AI engineer` | Comma-separated search queries |
| `ONSITE_LOCATIONS` | `Delhi,Mumbai,Bangalore` | Comma-separated locations |
| `ONSITE_JOB_BOARDS` | `indeed,glassdoor,linkedin` | Which boards to scrape |
| `REJECT_TITLES` | `senior,lead,manager,director` | Skip jobs with these words in the title |

See `.env.example` for the complete list with descriptions.

### Step 5: Add Your Resume

Place your resume PDF in the project root:

```bash
cp /path/to/YourResume.pdf resume.pdf
```

Or if your file has a different name, set `RESUME_FILE_PATH` in `.env`:

```
RESUME_FILE_PATH=ArinBalyan.pdf
```

This file is gitignored and never committed. It gets attached to every outreach
email.

### Step 6: Test With a Dry Run

Before sending real emails, run a dry run to verify everything works:

```bash
DRY_RUN=true uv run python -m jobspy_v2 onsite
```

**What to expect:**

1. The scraper will connect to job boards and download listings (this takes a
   few minutes depending on `RESULTS_WANTED`)
2. Results are written to the "Scraped Jobs" worksheet in your Google Sheet
3. For each job with a valid email, it will generate an email but NOT send it
4. Rows are updated with status "DryRun" in the spreadsheet
5. A summary report is emailed to your `REPORT_EMAIL`
6. You will see detailed logs in the terminal showing each step

**Check your Google Sheet after the run.** You should see:

- "Scraped Jobs" tab with all jobs and their status
- "Sent Emails" tab with the would-have-been-sent emails
- "Run Stats" tab with a summary row

If something fails, check the logs for error messages. Common issues:

| Error | Cause | Fix |
|-------|-------|-----|
| SMTP authentication failed | Wrong app password | Regenerate at myaccount.google.com/apppasswords |
| Google Sheets API error | Credentials issue | Make sure you shared the sheet with the service account email |
| No jobs with emails found | Normal for small searches | Increase `RESULTS_WANTED` or add more search terms |
| LLM request failed | API key issue | Check `OPENROUTER_API_KEY` -- fallback templates will be used automatically |

### Step 7: Go Live

Once your dry run looks good:

1. Set `DRY_RUN=false` in `.env` (or remove the line -- false is the default)
2. Run:
   ```bash
   uv run python -m jobspy_v2 onsite
   ```
3. Emails will be sent for real, with a 30-second pause between each one
4. Monitor the terminal and your Google Sheet for results

---

## Usage

### Run Modes

JobHunter supports two modes with separate search configurations:

```bash
# Scrape onsite/hybrid jobs (uses ONSITE_* settings in .env)
uv run python -m jobspy_v2 onsite

# Scrape remote jobs (uses REMOTE_* settings in .env)
uv run python -m jobspy_v2 remote
```

Each mode uses its own search terms, locations, boards, and limits defined in
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

### GitHub Actions (Full Setup)

GitHub Actions lets you run JobHunter automatically without keeping your
computer on. The repository includes three workflows:

| Workflow | File | What It Does |
|----------|------|-------------|
| `JobHunter Tests` | `tests.yml` | Runs the test suite on every push/PR to `main` |
| `JobHunter Onsite` | `onsite.yml` | Scrapes onsite jobs daily at 8:00 AM IST |
| `JobHunter Remote` | `remote.yml` | Scrapes remote jobs daily at 8:00 AM US Eastern |

#### Step-by-Step Setup

**1. Push the repo to GitHub (if not already done):**

```bash
git remote add origin https://github.com/YOUR_USERNAME/JobHunter.git
git push -u origin main
```

**2. Generate base64 strings for your private files.**

These files cannot be committed to git. You will paste the base64 output into
GitHub secrets.

```bash
# Resume PDF
base64 -w 0 resume.pdf
# Copy the ENTIRE output (it will be one long line)

# Applicant profile
base64 -w 0 contexts/profile.md
# Copy the ENTIRE output

# Google credentials JSON (if not already base64-encoded)
base64 -w 0 your-credentials-file.json
# Copy the ENTIRE output
```

On macOS, use `base64 -i` instead of `base64 -w 0`.

**3. Add repository secrets in GitHub.**

Go to your repo on GitHub, then **Settings > Secrets and variables > Actions >
New repository secret**. Add each of the following:

**Credentials:**

| Secret Name | Value |
|-------------|-------|
| `GMAIL_EMAIL` | Your Gmail address |
| `GMAIL_APP_PASSWORD` | Your Gmail App Password |
| `OPENROUTER_API_KEY` | Your OpenRouter API key |
| `GOOGLE_CREDENTIALS_JSON` | Base64 of your service account JSON (from step 2) |

**Files (base64-encoded):**

| Secret Name | Value |
|-------------|-------|
| `RESUME_BASE64` | Base64 of your resume PDF (from step 2) |
| `CONTEXT_BASE64` | Base64 of your `contexts/profile.md` (from step 2) |

**Identity:**

| Secret Name | Value |
|-------------|-------|
| `CONTACT_NAME` | Your full name |
| `CONTACT_EMAIL` | Your contact email |
| `CONTACT_PHONE` | Your phone number |
| `CONTACT_PORTFOLIO` | Your portfolio URL |
| `CONTACT_GITHUB` | Your GitHub username |
| `CONTACT_LINKEDIN` | Your LinkedIn URL |
| `CONTACT_CODOLIO` | Your Codolio username (or leave empty) |
| `RESUME_DRIVE_LINK` | Google Drive link to your resume |

**Storage:**

| Secret Name | Value |
|-------------|-------|
| `STORAGE_BACKEND` | `sheets` |
| `GOOGLE_SHEET_NAME` | Name of your Google Sheets spreadsheet |
| `REPORT_EMAIL` | Email address for run reports |

**LLM:**

| Secret Name | Value |
|-------------|-------|
| `LLM_BASE_URL` | `https://openrouter.ai/api/v1` |
| `LLM_MODEL` | Your model name (e.g., `google/gemini-2.0-flash-exp:free`) |
| `EMAIL_GENERATOR_MODE` | `llm` |

**Email Settings:**

| Secret Name | Value |
|-------------|-------|
| `APPLICATION_SENDER_NAME` | Your name as it appears in the From field |
| `FALLBACK_EMAIL_SUBJECT` | Fallback subject line if LLM fails |
| `FALLBACK_EMAIL_BODY` | Fallback email body if LLM fails |
| `REJECT_TITLES` | Comma-separated title filter patterns |
| `EMAIL_FILTER_PATTERNS` | Comma-separated email filter patterns |
| `MIN_EMAIL_WORDS` | `120` |
| `MAX_EMAIL_WORDS` | `300` |
| `EMAIL_INTERVAL_SECONDS` | `30` |

**Onsite Job Search Config:**

| Secret Name | Example Value |
|-------------|---------------|
| `ONSITE_SEARCH_TERMS` | `machine learning,data scientist` |
| `ONSITE_LOCATIONS` | `Delhi,Mumbai,Bangalore` |
| `ONSITE_JOB_TYPE` | `fulltime` |
| `ONSITE_JOB_BOARDS` | `indeed,glassdoor,linkedin` |
| `ONSITE_COUNTRY_INDEED` | `India` |
| `ONSITE_RESULTS_WANTED` | `300` |
| `ONSITE_MAX_EMAILS_PER_DAY` | `50` |

**Remote Job Search Config:**

| Secret Name | Example Value |
|-------------|---------------|
| `REMOTE_SEARCH_TERMS` | `machine learning,AI engineer` |
| `REMOTE_LOCATION` | `USA` |
| `REMOTE_IS_REMOTE` | `true` |
| `REMOTE_JOB_TYPE` | `fulltime` |
| `REMOTE_JOB_BOARDS` | `indeed,glassdoor,linkedin` |
| `REMOTE_COUNTRY_INDEED` | `USA` |
| `REMOTE_RESULTS_WANTED` | `300` |
| `REMOTE_MAX_EMAILS_PER_DAY` | `50` |

**4. Test it.**

Go to **Actions** in your GitHub repo. Click on **JobHunter Onsite** (or
Remote), then click **Run workflow**. Check the `dry_run` box, then click
**Run workflow**. Monitor the logs to make sure everything works.

**5. Enable automatic scheduling.**

The workflows are already configured with cron schedules. Once your test run
passes, the scraper will run automatically every day. You can edit the cron
times in `.github/workflows/onsite.yml` and `remote.yml`.

#### How File Secrets Work

The workflows automatically decode your base64 secrets into files before
running:

```
RESUME_BASE64  -->  decoded to  -->  resume.pdf (project root)
CONTEXT_BASE64 -->  decoded to  -->  contexts/profile.md
```

This happens in a "Decode private files" step in each workflow. The decoded
files exist only for the duration of the workflow run and are never stored in
the repository.

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
  contexts/
    profile.md           # Your applicant profile (gitignored)
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
| Profile | `CONTEXT_FILE_PATH` (default: `contexts/profile.md`) |
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

- [python-jobspy](https://github.com/SpeedyApply/JobSpy) by
  [SpeedyApply](https://github.com/SpeedyApply) -- the multi-board job scraping
  engine that powers JobHunter's data collection
- [OpenRouter](https://openrouter.ai/) -- LLM API gateway used for email
  generation
- [gspread](https://github.com/burnash/gspread) -- Google Sheets API client for
  Python

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for
details.
