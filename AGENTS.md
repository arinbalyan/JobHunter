# Agent Configuration

## Project Status
**Complete.** All phases done. Rust rewrite of the original Go JobHunter, using a thin Go bridge to call [scrappy](https://github.com/arinbalyan/scrappy) for job scraping.

---

## Roadmap

### Phase 1: Scaffold + Config + DB ✅ (2026-06-26)
- [x] Rust project skeleton — Cargo.toml, clap subcommands
- [x] config.toml parsing — serde + toml + $VAR env resolution
- [x] Postgres connection — sqlx + embedded migrations
- [x] Initial schema — jobs, email_queue, tracking, run_log tables
- [x] Go scraper subprocess — stdin JSON → scrappy.ScrapeJobs() → stdout JSON
- [x] `./jobhunter doctor` — checks config, DB URL, scraper binary, LLM keys

### Phase 2: Scrape Workflow ✅ (2026-06-26)
- [x] Rust serializes search params to JSON, spawns Go scraper
- [x] Deserialize scraper stdout → Vec<JobPost>
- [x] Title rejection filter (~40 patterns)
- [x] Gentle email filter (no-reply@, do-not-reply@, suspicious TLDs)
- [x] Atomic SQL dedup — INSERT WHERE NOT EXISTS by job_url
- [x] Email queue population per job
- [x] `results_wanted: 0` (unlimited) — scrappy returns all jobs
- [x] `timeout_seconds` from config — bridge uses `[scrape].max_runtime_minutes`
- [x] Pending job carry-over — INSERT-SELECT for unqueued jobs from last 7 days
- [x] Telegram report — scrape summary with timing, mode, sites/terms
- [x] GH Actions — 4x daily scrape workflow
- [x] Onsite/remote mode — `--mode remote|onsite`

### Phase 3: Send Workflow ✅ (2026-06-26)
- [x] LLM router — weighted random + failover (3 attempts, 30s cooldown)
- [x] `/models` discovery — checks all 9 providers at startup
- [x] Email generation via LLM (prompts from config.toml)
- [x] Template fallback when all providers fail
- [x] Concurrent generation — semaphore (default 10)
- [x] SMTP sender — lettre, Gmail 587 STARTTLS
- [x] Rate-limited send — delay_seconds between sends
- [x] Quota tracking — daily sent count, stops at daily_limit
- [x] Tracking pixel injection — 1x1 img tag in HTML body
- [x] URL click tracking — wraps signature links with /click?e=&url= redirects
- [x] GH Actions — daily send workflow
- [x] Signature footer with GitHub, Portfolio, Resume links

### Phase 4: Tracker + Notifications ✅ (2026-06-26)
- [x] HTTP tracking server — axum: /track, /click, /health, /version
- [x] Vercel deployment — Node.js serverless functions at jobhunter-tracker.vercel.app
- [x] Open tracking — 1x1 GIF pixel, logs to DB
- [x] Click tracking — /click?e=&url= logs to click_log, 302 redirect
- [x] Pipeline dashboard — https://jobhunter-tracker.vercel.app/
- [x] Run log persistence — write_run_log() after scrape and send
- [x] Telegram alerts — rich scrape report per run

### Phase 5: Polish + Deploy ✅ (2026-06-27)
- [x] LLM job scoring (1-10) — `./jobhunter score`
- [x] LLM company research (3 talking points) — `./jobhunter research`
- [x] LLM reply triage — `./jobhunter triage` classifies recruiter replies
- [x] GH Actions: scrape (4x daily) + send (daily) + tests (on push)
- [x] Release packaging — `scripts/build-release.sh`
- [x] README with quick-start
- [x] Vercel dashboard with full pipeline stats, per-URL click breakdown

---

## scrappy Integration

scrappy is at `~/projects/scrappy/` (v0.3.5, 141 sites, 49 working). Consumed as a Go library import via the thin `scraper/main.go` bridge (~30 lines).

**📝 scrappy improvements**: Found limitations go in `scrappy_improvements.md` (gitignored).

## LLM Providers (All Free Tier)

| Provider | Complex Model | Weight |
|----------|--------------|--------|
| OpenRouter | google/gemma-4-31b-it:free | 10 |
| Groq | openai/gpt-oss-120b | 5 |
| Together | google/gemma-4-9b-it | 4 |
| DeepInfra | Llama-3.3-70B-Instruct-Turbo | 4 |
| Hyperbolic | qwen3-coder-480b-a35b-instruct:free | 3 |
| SambaNova | Meta-Llama-3.3-70B-Instruct | 3 |
| Cerebras | zai-glm-4.7 | 2 |
| Z.AI | GLM-4-Plus | 1 |

## Quick Reference

```
jobhunter scrape --mode remote|onsite    # Scrape → filter → dedup → queue
jobhunter score                          # Score unscored jobs 1-10
jobhunter research                       # Generate 3 talking points per company
jobhunter send                           # Generate + send emails
jobhunter triage "<reply text>"          # Classify recruiter reply
jobhunter serve                          # Tracking server + dashboard
jobhunter doctor                         # Diagnose everything
```
