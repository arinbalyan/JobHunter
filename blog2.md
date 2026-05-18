# JobHunter: Building a Production Job Scraping & Cold Email Pipeline

## The Problem

Applying to jobs manually is a volume game. You need to review hundreds of listings, find recruiter emails, write personalized cover letters, track what you've sent, and avoid double-applying. It's an operations problem disguised as a job search.

The metric I owned was yield — of N jobs scraped, how many become real, personalized emails sent? That yield is the product of: scrape completeness → email extraction rate → LLM generation success rate → deliverability check pass rate → dedup pass rate.

Built over ~3 months (Feb → May 2026, 52 commits), open-sourced as JobHunter.

---

## Architecture

```
CLI (onsite / remote / serve)
  └─ BaseWorkflow (template method)
      ├─ Phase 1: scrape + batch-write to Sheets (status: Pending)
      ├─ Phase 2: for each job → validate DNS → dedup → LLM email → send → update row
      └─ Phase 3: carry-over pending jobs from previous runs
```

| Layer | What | Why |
|-------|------|------|
| `config/settings.py` | Pydantic `BaseSettings` — every field from env | Zero hardcoded values, testable via env override |
| `core/scraper.py` | `ThreadPoolExecutor` + `python-jobspy` | I/O-bound HTTP calls, parallelize across term×board×location |
| `core/email_gen.py` | LLM (OpenRouter) → fallback template | Cost control: free tier Qwen model, graceful degradation |
| `core/dedup.py` | Two rules: exact match + company/day | Prevents embarrassing double-sends |
| `utils/email_utils.py` | Regex validation + DNS/MX via `emval` | Filters 30%+ of scraped emails before they reach the sender |
| `storage/` | Protocol → SheetsBackend / CsvBackend | Swap at runtime via env var. Sheets gives audit trail across CI runs |
| `.github/workflows/` | Two scheduled pipelines (onsite IST, remote ET) | 350-min timeout, 200 parallel scrape workers |

### Why Google Sheets over a database?
Free, no infra. I can open it on my phone during the day and see what was sent. The Row Number = primary key pattern is unusual but works for carry-over processing. Downside: cell-by-cell `update_cell` has API rate limits — acceptable at 50-200 emails/day.

### Why python-jobspy?
Multi-board scraping (Indeed, LinkedIn, Glassdoor, Google, ZipRecruiter) in one library, returns structured DataFrames with email fields. Trade-off: we depend on their rate-limit handling, and Pydantic v1/v2 deprecation warnings were noise we had to suppress.

---

## The Optimization Loop

### Cycle 1: Worker Scaling
- **Before:** `SCRAPE_MAX_WORKERS = 5`, scraping took 45+ minutes for 1000 results across 3 boards × 3 locations × 3 terms (27 tasks)
- **Diagnosis:** `ThreadPoolExecutor` was massively underutilized. GitHub Actions runners have 2+ vCPUs and these are HTTP I/O calls
- **Ruled out:** Python GIL (not relevant for I/O), jobspy internals (not our bottleneck)
- **Fix:** 5 → 50 → 150 → **200 workers**
- **Result:** Scrape time dropped to 5-8 minutes

### Cycle 2: Email Quota Tuning
- Started at 50/day → 250 → 500 — hit Gmail's 500/day sending limit hard
- **Diagnosis:** Gmail quota errors in logs. Added `is_gmail_quota_error()` detection and graceful stop
- **Fix:** Added `daily_total_emails_limit = 500` with carry-over processing — if you hit quota mid-run, remaining jobs get status "Pending" and process next day

### Cycle 3: The Cartesian Explosion Bug
Remote mode had `locations × countries_indeed` as nested loops producing a Cartesian product. 3 locations × 3 countries = 9 pairs with invalid combos like `location="India" + country_indeed="UK"`.
- **Fix:** `_get_remote_location_country_pairs()` uses `zip()` instead of nesting. When `REMOTE_IS_REMOTE=true`, uses `[("Remote", c) for c in countries]` — 1:1, not N×M
- **Result:** 9 tasks → 3 tasks. 3x improvement in scrape time for remote mode

### Cycle 4: DNS/MX Email Validation
- **Problem:** 20-30% of scraped emails were to non-existent mailboxes
- **Diagnosis:** Gmail returning mailer-daemon failures. Regex alone can't detect if a domain accepts mail
- **Solution:** Added `emval` (Rust-based DNS MX lookup) as a deliverability gate. Overhead is negligible since we already wait 30s between sends
- **Metric:** Filters 10-15% of apparently-valid emails

### Cycle 5: Workflow-Specific Fallback Bodies
- Initially had one generic fallback template. Onsite (India) and remote (global) jobs need different messaging
- Added `onsite_fallback_email_body` and `remote_fallback_email_body` with `workflow_mode` routing

---

## What Broke

1. **Gmail quota at 500/day** — hard limit, no workaround without multiple accounts. Carry-over system was a pragmatic patch.
2. **Resume GPG encryption** — initial design encrypted the resume for GitHub Actions. Added complexity (passphrase management, decryption step). Switched to committing the PDF directly with `.gitignore` exception.
3. **Pydantic deprecation noise** — `python-jobspy` uses Pydantic v1 internally, triggering warnings in worker threads. Had to suppress by `message=` pattern (warnings fire from jobspy threads, not our module).
4. **Base64 padding on GitHub secrets** — some secrets had missing padding. Added auto-padding logic.

## What I'd Do Differently

1. **Database instead of Sheets.** Postgres would let me query trends ("which boards produce the most valid emails?") and do real-time dashboards.
2. **Batch email validation.** Currently DNS validation happens inline per-email. Parallelize DNS lookups as a pre-filter step before Phase 2.
3. **A/B test LLM prompts.** One prompt template currently — would test subject lines, personalization depth, and temperature settings to optimize reply rate.
4. **Better dedup.** Current dedup is in-memory cached from Sheets. A real database would enable cross-session dedup with no data loss.
5. **Monitoring.** No alerting if the pipeline crashes on a weekend. GitHub Actions notifications are the only feedback loop.

---

## Before / After Metrics

| Metric | Before | After |
|--------|--------|-------|
| Scrape time | 45+ min | 5-8 min |
| Emails/day | 50 | 500 (Gmail cap) |
| Scrape workers | 5 | 200 |
| Remote task explosion | 9x | 1x (zip) |
| Bounce rate | ~20% | ~5% |
| Test count | 0 | 200 |
| Pipeline timeout | 30 min | 350 min |
