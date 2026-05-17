# Building JobHunter: An Automated Job Scraping & Cold Email Outreach System

*Published: May 2025 — by Arin Balyan*

---

## Introduction

Job searching is a numbers game. In a competitive market, sending out a handful of applications manually just doesn't cut it. What if you could automatically scrape every relevant job listing across multiple boards, extract recruiter contact emails, generate personalized cold emails, and send them — all while you sleep?

That's exactly what **JobHunter** does. This blog post walks through how the project was built, the challenges faced, the architectural decisions made, and the evolution from a simple scraper into a production-grade, fully automated outreach system running on GitHub Actions.

---

## The Origin: A Problem Worth Solving

The project started with a simple frustration: job boards like Indeed, LinkedIn, and Glassdoor each have their own interfaces, and manually applying to roles is slow. The goal was to build a tool that could:

1. Search multiple job boards simultaneously
2. Extract recruiter or hiring manager email addresses
3. Generate personalized cold emails using an LLM
4. Send outreach emails with a resume attached
5. Track everything in a shared, persistent database
6. Run on a schedule without keeping a local machine on

What began as a weekend project grew into a serious piece of infrastructure.

---

## Timeline of Development

### Phase 1: Foundation (Feb 2025)

The initial commit on **February 14, 2025** marked the first stable release (v1.0.0). The core architecture was laid out with clear separation of concerns:

```
src/jobspy_v2/
  config/      — Settings (Pydantic) and defaults
  core/        — Scraper, email generation, sending, deduplication, reporting
  storage/     — Abstract storage protocol with Sheets and CSV backends
  workflows/   — Onsite and remote pipeline orchestration
  scheduler/   — APScheduler-based cron scheduling
  utils/       — Email and text utilities
```

Key decisions made in this phase:

- **python-jobspy** was chosen as the scraping engine. It already supports Indeed, LinkedIn, Glassdoor, and more, abstracting away the board-specific scrapers.
- **Pydantic Settings** (via `pydantic-settings`) was used for all configuration — everything comes from environment variables. No hardcoded secrets, ever.
- **Two-phase pipeline**: Scrape everything first, save it, then process and email. This protects against data loss if the process crashes mid-run.
- **Dual storage backends**: Google Sheets for production (audit trail, shared visibility) and CSV for local testing.

### Phase 2: LLM-Powered Email & Polish (Feb 14–15, 2025)

The second wave of work focused on making the outreach emails feel genuinely personal rather than generic templates. OpenRouter was chosen as the LLM gateway because:

- It provides a unified API across dozens of models
- Free-tier models are available for testing
- The `OpenAI` Python client works out of the box with a custom base URL

The email generation flow:
1. Load the applicant's profile from `contexts/profile.md`
2. Form a prompt with the profile, job title, company, and job description
3. Call the LLM via OpenRouter
4. Parse the response (SUBJECT: line + body), clean artifacts, truncate to word count
5. Append a contact-info footer with optional resume link
6. If the LLM fails at any point, silently fall back to a configurable text template

This was deliberate: the fallback ensures emails still go out even if the API is down.

### Phase 3: Scaling Up (Feb 15–23, 2025)

Once the core was working, the focus shifted to throughput and reliability:

- **Scrape workers increased** from 50 to 200 to speed up parallel job fetching across boards, locations, and search terms.
- **Email limits increased** from 250 to 500 per day, giving the system more runway before hitting Gmail's daily quota.
- **Workflow timeout bumped** from 30 minutes to 360 minutes (6 hours, the GitHub Actions hard limit), matching the needs of large-scale scraping.
- **Pydantic deprecation warnings suppressed** in three layers — the `jobspy` library was calling a deprecated `.dict()` method that fired warnings from jobspy's worker threads. Fixing this required suppression at the module level (`__main__.py`), the jobspy source (`a98862e`), and globally (`a1d44d1`).

### Phase 4: DNS Email Validation (Feb 25, 2025)

One of the trickiest problems in cold outreach is bounce rate. Sending to `hr@company.com` or worse, `noreply@company.com`, not only wastes quota but tanks sender reputation.

The fix: **emval**, a Rust-based DNS/MX lookup library. Before sending, every extracted email address is validated against DNS records to confirm its mailbox exists. This eliminates generic inboxes that cause bounce-backs.

The commit `cbd7fc2` also introduced a second important change: **workflow-specific fallback email bodies**. Instead of one generic fallback template, onsite and remote runs can use different pre-written bodies — making the fallback path feel more tailored.

### Phase 5: Remove GPG Encryption (Feb 28, 2025)

The project initially stored the resume PDF as a GPG-encrypted file (`ArinBalyan.pdf.gpg`), decrypting it at runtime in GitHub Actions. This added complexity and a passphrase secret to manage.

That changed in two commits:
- `05005d9`: Removed GPG encryption — the password field was deleted from workflows.
- `464de0a`: The resume PDF was committed directly to the repo. The `.gitignore` was updated with an explicit `!ArinBalyan.pdf` exception so git tracks it.

Offsetting this was a clear trade-off: the resume is now visible to anyone with repo access. For a personal project, this权衡 was acceptable. For a larger team, the previous approach would be safer.

### Phase 6: Debug Time Parsing & Remote Job Fixes (Feb 23 – Mar 2025)

A subtle but critical bug emerged in the remote scraping workflow. The environment allowed specifying multiple locations and multiple countries for Indeed, but the pairing logic was using nested loops, creating a **Cartesian product** — every location combined with every country — leading to `N × M` tasks instead of the intended 1:1 zip mapping.

This was fixed in `dfb07c0` by replacing the nested loop with `zip()`, then refined in a subsequent commit to add a length-mismatch guard and safe fallback.

On the same day (`15cbedd`), jobspy time format parsing for `date_posted` was failing on formats like `7 hours ago` and `1 day ago` because `datetime.fromisoformat()` can't parse those relative strings. This was adjusted to handle the case gracefully.

### Phase 7: Glassdoor Country Support Check (Apr 2026, PR #2)

The most recent work involved a cross-country scraping issue. Glassdoor's structure varies by country — some country codes aren't supported by python-jobspy's `Country` enum. Rather than silently failing or crashing, the scraper now:

1. Checks `Country.__members__` for the country code
2. Inspects the Country enum's value tuple (3rd element = Glassdoor TLD/subdomain)
3. Skips the board with a debug log message when unsupported

This fix was part of **PR #2**, which went through multiple review iterations (`copilot-swe-agent`) addressing logging quality, country parameter naming, and test clarity.

---

## Architecture Deep Dive

### The Two-Phase Pipeline

The entire system runs as a two-phase pipeline in `BaseWorkflow.run()`:

```
Phase 1 — Scrape & Save
  search_terms x locations x boards → ThreadPoolExecutor → pandas → Google Sheets
  Every saved row starts with email_sent = "Pending"

Phase 2 — Process & Email
  For each row:
    1. Extract deliverable emails (with DNS validation via emval)
    2. Check dedup rules (exact match + company daily cooldown)
    3. Generate email (LLM → OpenRouter → fallback template)
    4. Send via SMTP with resume attached
    5. Update row status: Yes / Skipped / Failed / DryRun

Phase 3 — Carry-over Pending Jobs
  Check daily quota remaining → process unfulfilled Pending rows
```

This design is deliberately resilient. If Phase 2 crashes, all jobs are still saved in Phase 1 and will be picked up on the next run.

### Deduplication: Three Guards Against Spam

The `Deduplicator` class enforces three rules:

| Rule | Scope | Description |
|------|-------|-------------|
| Exact email match | Permanent | Never send to the same email address twice |
| Company daily cooldown | Per day | Never email two different people at the same company on the same day |

Every sent email is recorded in the "Sent Emails" worksheet, which feeds the in-memory cache for fast lookups during a run.

### Storage: Protocol-Driven Design

The `StorageBackend` Protocol in `storage/base.py` defines the contract. Both backends (`SheetsBackend` and `CSVBackend`) implement:

- `get_sent_emails()` — for dedup lookups
- `add_scraped_jobs()` — batch write with row-number tracking
- `update_scraped_job_status()` — per-row status update (critical for crash resilience)
- `get_pending_jobs()` — fetch unprocessed rows from previous runs
- `get_today_sent_emails_count()` — shared daily quota tracking

The Sheets backend handles credential parsing robustly: it auto-pads base64 strings (fixed in `a3fc797`) and falls back from base64 to raw JSON.

### Scraper: Smart Board-Aware Param Handling

The `scraper.py` engine is particularly nuanced. Each job board has different API constraints:

- **Indeed**: Only ONE of `hours_old` / `job_type+is_remote` / `easy_apply` per request. The code prioritizes `job_type+is_remote` over `hours_old`.
- **Google**: Uses `google_search_term` instead of `search_term`.
- **LinkedIn**: `easy_apply` filter no longer works — stripped at param adaptation time.
- **Glassdoor**: Country code validated before scraping; unsupported countries are skipped with a debug log.

Remote location-country pairing was a major source of bugs. The fix ensures a strict 1:1 zip with a safe fallback to avoid the Cartesian explosion that previously caused `N × M` tasks.

---

## GitHub Actions: The Real Deployment

The real power of JobHunter comes from its GitHub Actions workflows. There are three workflows:

| Workflow | Cron | Purpose |
|----------|------|---------|
| `onsite.yml` | UTC 2:00 AM (IST 7:30 AM) | Daily onsite/hybrid job scrape |
| `remote.yml` | UTC 13:00 (US Eastern 8:00 AM) | Daily remote job scrape |
| `tests.yml` | On every push/PR | Full test suite with pytest |
| `onsite.yml` | workflow_dispatch | Manual trigger with dry-run option |

Key configuration details:
- **Concurrency groups**: Cancel in-progress runs before starting new ones to prevent duplicate emails.
- **Max timeout**: 350 minutes (5h 50m) — leaving a 10-minute buffer before GitHub's 6-hour hard limit.
- **Secrets management**: Everything sensitive is a GitHub Secret — no `.env` file needed in the repo.

---

## Test Suite: 200 Tests of Confidence

The test suite lives in `tests/unit/` and is organized by layer:

| File | Coverage |
|------|----------|
| `test_config.py` | Settings parsing, CSV splitting, validation, default application |
| `test_storage.py` | Sheets backend, credential parsing, auto-retry, row tracking |
| `test_core.py` | Scraper, email gen (LLM + fallback), dedup, reporter |
| `test_workflows.py` | Pipeline orchestration, pending job carry-over, title/email filtering |
| `test_utils.py` | Email extraction, text cleaning, word counting |
| `test_scheduler.py` | Cron expressions, runner exceptions |

All tests run with `uv run pytest` with `-v --tb=short` for readable output. Coverage is measured with `uv run pytest --cov=jobspy_v2`. Linting is handled by Ruff with a line length of 88 and Python 3.10 as the target.

---

## Key Lessons Learned

1. **Two-phase isn't just a pattern, it's a safety net.** Saving scraped data before processing email means a crash costs you processing time, not data. The carry-over logic ensures no pending job is silently dropped.

2. **The resume encryption trade-off.** Storing the resume unencrypted in the repo is simple and avoids decryption overhead, but it's not secure. For a personal project it's fine; for anything shared, encryption is non-negotiable.

3. **GeoNames and country handling is subtle.** Indeed's country parameter, Glassdoor's country support detection, and the remote location-country pairing each required careful investigation of the python-jobspy source. The `Country.__members__` check and the 1:1 zip fix are examples of handling library-specific quirks.

4. **DNS validation (emval) matters beyond spam reduction.** Email deliverability doesn't just affect noise — it affects your sending reputation, which directly impacts whether Gmail places your future emails in inbox or spam.

5. **Fetch-desc does not equal fetch-desc on LinkedIn.** One line — `linkedin_fetch_description=True` in scraper params — determines whether the LinkedIn scraper retrieves the full job description. Without it, the LLM can't write personalized emails, and the fallback takes over silently.

---

## Project Stats at a Glance

| Metric | Value |
|--------|-------|
| Total commits | 40+ |
| Release | v1.0.0 (MIT License) |
| Author | Arin Balyan |
| Language | Python 3.10+ |
| Dependencies | python-jobspy, OpenRouter, pandas, gspread, apscheduler, emval |
| Test files | 6 files, ~200 tests |
| GitHub Actions workflows | 3 (CI + 2 scheduled scrapes) |
| Storage backends | 2 (Google Sheets, CSV) |
| Modes | 2 (onsite, remote) |
| Boards scraped | Indeed, LinkedIn, Glassdoor (configurable) |
| Scrape parallelism | 200 ThreadPoolExecutor workers |

---

## What's Next

The project is feature-complete for its core purpose, but there are natural next directions:

- **Per-board email extraction strategies**: Different boards expose emails differently — a registry of extraction strategies per board could improve yield.
- **ATS integration**: Filing applications through Greenhouse, Lever, or other ATS platforms rather than cold email.
- **Unsubscribe compliance**: Following CAN-SPAM/GDPR even for cold outreach requires unsubscribe links and opt-out handling.
- **Feedback loop**: Tracking reply rates and using them to iterate on email template quality.

---

*The source is available at [github.com/arinbalyan/JobHunter](https://github.com/arinbalyan/JobHunter).*

---

**Tags:** `python` `job-scraping` `cold-email` `automation` `github-actions` `gspread` `openrouter` `apscheduler`
