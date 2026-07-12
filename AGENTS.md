# Agent Configuration — JobHunter

## Project Overview

Fully automated Rust job application pipeline. Scrapes 141 job boards via [scrappy](https://github.com/arinbalyan/scrappy) (Go library through a thin Go bridge), scores/researches jobs via LLM (9 free-tier providers with failover), generates personalized outreach emails, and tracks opens/clicks via a Vercel dashboard.

**Cost**: $0 (all LLM providers are free tier, NeonDB free tier, Vercel Hobby, GH Actions free)

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   jobhunter (Rust)                    │
│  ┌──────────┐  ┌──────────┐  ┌────────┐  ┌───────┐  │
│  │ scrape   │  │ send     │  │ score  │  │ serve │  │
│  │ score/   │  │ generate │  │research│  │tracker│  │
│  │ research │  │ → SMTP   │  │triage  │  │:8080  │  │
│  └────┬─────┘  └────┬─────┘  └────────┘  └───────┘  │
│       │              │                                │
│  ┌────▼─────┐  ┌────▼─────┐                          │
│  │ Go bridge│  │ LLM      │  9 free providers         │
│  │ scraper/ │  │ router   │  weighted + failover      │
│  │ main.go  │  │ src/llm  │                           │
│  └────┬─────┘  └──────────┘                           │
│       │              │                                │
└───────┼──────────────┼────────────────────────────────┘
        │              │
  ┌─────▼─────┐  ┌─────▼──────┐    ┌───────────────────┐
  │ scrappy   │  │ PostgreSQL │    │ Vercel Dashboard  │
  │ (Go lib)  │  │ (NeonDB)   │    │ jobhunter-tracker │
  │ 141 sites │  │ jobs,      │    │ .vercel.app       │
  │ 49 working│  │ email_queue│    │ /track /click     │
  └───────────┘  │ tracking,  │    └───────────────────┘
                 │ run_log    │
                 └────────────┘
```

**Data flow**: scrappy (Go) → JSON stdout → Rust parses → filters titles/emails → INSERT WHERE NOT EXISTS → queues emails → LLM generates → SMTP sends → tracking pixels track opens/clicks.

---

## Branch & CI Structure

```
dev  ──sync-all.sh──►  beta  ──merge PR──►  main  ──release.yml──►  v0.1.x
  ▲                      ▲                     │
  │                      │                     ├─ scrape.yml (4x daily)
  └── push only ─────────┘                     │    remote + onsite modes
                                                ├─ send.yml (daily)
                                                └─ tests.yml (on push)
```

- **Work on `dev`**, run `./sync-all.sh` to promote to beta (auto-merge PR) → main (manual merge PR)
- **Releases** auto-bump patch version on push to main
- **GH Actions**: scrape/download tarball from latest release (not build from source)

---

## Key Files

| File | Purpose |
|------|---------|
| `src/main.rs` | CLI entry — 8 subcommands |
| `src/scrape.rs` | Scraper bridge, JSON parsing, filters, Location struct |
| `src/send.rs` | Email pipeline — fetch pending, generate, send |
| `src/smtp.rs` | SMTP sender via lettre, tracking pixel/URL wrapping |
| `src/llm.rs` | LLM router — 9 providers, weighted random, failover |
| `src/config.rs` | TOML config with `$VAR` env resolution |
| `src/telegram.rs` | Rich Telegram scrape report |
| `src/db.rs` | Postgres connect, migrate, write_run_log |
| `scraper/main.go` | ~30 line Go bridge — stdin→scrappy→stdout |
| `scraper/go.mod` | References scrappy GitHub module version |
| `api/dashboard.js` | Vercel dashboard — dark mode, pipeline stats, run history |
| `config.ci.toml` | Committed config for CI (real terms/sites, placeholder secrets) |
| `config.toml` | Local personal config (gitignored) |
| `.github/workflows/scrape.yml` | 4x daily scrape + Telegram report |
| `.github/workflows/send.yml` | Daily email send (disabled currently) |

---

## Current State (2026-07-03)

### Scraper
- **scrappy v0.3.10** with full email enrichment pipeline:
  - ExtractFromHTML (mailto: + inline regex from descriptions)
  - EmailEnrich (auto-generates hr@/careers@/recruiting@/jobs@{domain})
  - Domain-level batch enrichment (visits each company website once, probes /about /contact /team /careers)
  - Company name → domain heuristic + multi-TLD fallback
  - Skips personal domains (gmail/outlook/yahoo/hotmail/aol)
- **Location struct fix**: scrappy returns location as JSON `{city,state,country}`. Rust matches it exactly now (was expecting `Option<String>`, causing JSON parse error)
- **Email yield**: Before v0.3.10: 86 emails / 112k listings = 0.08%. Expected with domain enrichment: 2,000-5,000 (2-5%)
- **Remote locations**: 11 (remote, global, international, worldwide, EMEA, APAC, Americas, US, UK, Canada, Europe)
- **Onsite scraping**: Added to GH Actions workflow (runs after remote)

### Dashboard (jobhunter-tracker.vercel.app)
- Full dark mode with oklch colors
- Pipeline funnel: Raw → Filtered/Deduped → Unique jobs → Unique email+company
- Per-site breakdown, score distribution, run history, failures
- Fixed: bigint string-concat bug, -0 display, "Queued"→"Inserted" label

### Send workflow
- **Currently disabled** (waiting for scrape to fill meaningful email queue)
- LLM router with 9 free providers, weighted random + failover
- Tracked emails via tracking pixel + click redirect wrapping

### Email-rich sites (top yielders)
| Site | Jobs | Why it works |
|------|------|-------------|
| Greenhouse | 719 | ATS, no emails — EmailEnrich + domain enrichment now fills the gap |
| Indeed | 133 | Has some emails, now enhanced by domain enrichment |
| mycareersfuture | 63 | Natively exposes recruiter emails |
| himalayas | 17 | Natively exposes recruiter emails |

---

## Key Design Decisions

- **Config is data, not code**: All filters (title reject, email block), search terms, LLM prompts in `config.toml`. Change without recompiling.
- **scrappy config auto-load**: `scrappy_config = "~/projects/scrappy/config.toml"` loads per-site search terms automatically.
- **Pre-built tarball in CI**: `gh release download` gets latest release. No Rust/Go toolchain in scrape/send workflows. Tests still build from source.
- **All filters in config**: `reject_titles`, `blocked_email_prefixes`, `blocked_email_contains`, `blocked_tlds` — no hardcoded Rust arrays.
- **EmailEnrich enabled**: `email_enrich: true` in bridge input (scrappy v0.3.10).
- **Don't spam**: Email queue dedup prevents same (email+company) being queued within 30 days.
- **No sync-all automation**: User runs it manually. Always push to `dev` only.

---

## Improvement Docs (read before modifying)

- **`scrappy_improvements.md`** — scrappy-side improvements backlog. 16/17 original done, 4/4 email extraction gaps fixed. Items that scrappy should fix, not JobHunter.
- **`jobhunter_improvements.md`** — JobHunter-side improvements backlog. 9 items (per-site stats, telegram reports, config validation, etc.). Items that JobHunter should fix, not scrappy.

Both cross-reference each other. If you find a bug, determine which project should fix it.

---

## Quick Reference

```bash
./jobhunter scrape --mode remote|onsite    Scrape → filter → dedup → queue
./jobhunter score                          Score unscored jobs 1-10
./jobhunter research                       Research 3 talking points per company
./jobhunter send                           Generate + send emails
./jobhunter triage "<reply text>"          Classify recruiter reply
./jobhunter import --from ~/projects/scrappy/config.toml  Import scrappy per-site config
./jobhunter serve                          Tracking server + dashboard
./jobhunter doctor                         Diagnose everything
```

## Session Context 2026-07-03

All changes on `dev` branch. Not yet promoted to main. Run `./sync-all.sh` then merge the beta→main PR.

### What was done
- **scrappy v0.3.7 → v0.3.10**: Multiple bumps. v0.3.10 includes ExtractFromHTML, EmailEnrich, domain-level batch enrichment, company→domain heuristic.
- **Location struct fix** (`src/scrape.rs`): scrappy returns `location` as JSON object `{city,state,country}`. Added matching Rust struct.
- **Dashboard** (`api/dashboard.js`): Full dark mode oklch redesign, fixed bigint string-concat bug (showed 860k instead of 86), added cumulative pipeline stats, fixed -0 display.
- **Expanded locations** (`config.ci.toml`): 1→11 remote locations (global, EMEA, APAC, Americas, US, UK, Canada, Europe, etc.)
- **Onsite scraping** (`.github/workflows/scrape.yml`): Added `--mode onsite` step after remote.
- **EmailEnrich enabled** (`src/scrape.rs`): Added `email_enrich: true` to bridge input.
- **Software eng search terms** (`config.ci.toml`): Added Backend, Frontend, Full Stack, React, Node, Go, Rust, TypeScript, DevOps, Platform, etc.
- **Release tag race fix** (`.github/workflows/release.yml`): While-loop to find next free tag.
- **Download fix** (`.github/workflows/scrape.yml`): `cp` instead of `mv` to avoid directory conflict with `scraper/`.

### Files Changed
| File | Change |
|------|--------|
| `src/scrape.rs` | Location struct, dedup_skipped, email_enrich, emails_only revert |
| `src/telegram.rs` | dedup_skipped in Telegram scrape report |
| `scraper/go.mod` | v0.3.7→v0.3.8→v0.3.9→v0.3.10 |
| `api/dashboard.js` | Dark mode redesign, bigint fix, cumulative pipeline stats |
| `config.ci.toml` | 11 remote locations + software eng search terms |
| `.github/workflows/scrape.yml` | Onsite scrape step + download fix |
| `.github/workflows/release.yml` | Tag race condition fix |
| `scrappy_improvements.md` | Email investigation, 4/4 fixes done |
| `jobhunter_improvements.md` | Created with 9 items |
| `AGENTS.md` | This file — full agent context |

### What's pending
- Merge PR #38 (beta→main) to deploy dashboard + new scrape config to GH Actions
- After merge, release v0.1.5 will trigger with all changes
- Send workflow still disabled — waiting to verify email yield from v0.3.10's domain enrichment

## Session Context 2026-07-07

### Problem: scrape timeout
All scrape runs hit the step-level `timeout-minutes: 300` (not the 6h job timeout).
`max_runtime_minutes = 330` was *higher* than the step timeout, so GH killed the process
before scrappy's context.WithTimeout could fire and return partial results gracefully.

### Changes made (all on dev, committed as 3693578)

**Time-based graceful shutdown** (`config.ci.toml`):
- `max_runtime_minutes: 330 → 290` — 10min buffer below GH's 300min step timeout
- scrappy's context expires at 290min, returns partial results, Rust processes them
- `results_wanted = 0` stays (unlimited, now time-bounded instead)

**Split remote/onsite workflows**:
- `scrape.yml` → deleted
- `scrape-remote.yml` (new) — concurrency group `scrape-remote`
- `scrape-onsite.yml` (new) — concurrency group `scrape-onsite`
- Each runs independently on separate runners, no DB conflicts

**Per-mode email config** (separate LLM context + templates for remote vs onsite):
- Migration: `ALTER TABLE jobs ADD COLUMN scrape_mode TEXT`
- `src/config.rs`: `context_remote`/`context_onsite` on User; template overrides on Templates
- `src/scrape.rs`: pass mode string to `insert_job`, store in `scrape_mode` column
- `src/send.rs`: fetch `scrape_mode` via JOIN, select per-mode context/template in `generate_one`
  Falls back to default when override is unset

### Files Changed
| File | Change |
|------|--------|
| `config.ci.toml` | max_runtime 330→290, comment updated |
| `.github/workflows/scrape.yml` | deleted |
| `.github/workflows/scrape-remote.yml` | created (remote only, separate concurrency) |
| `.github/workflows/scrape-onsite.yml` | created (onsite only, separate concurrency) |
| `migrations/20260707000005_scrape_mode.sql` | created — add scrape_mode to jobs |
| `src/config.rs` | context_remote, context_onsite; template overrides |
| `src/scrape.rs` | pass/store scrape_mode in insert_job |
| `src/send.rs` | fetch scrape_mode, select per-mode context/templates |

### What's pending
- Actually configure different context/templates for remote vs onsite in config.ci.toml if desired
  (currently both fall back to the shared defaults)
- Rebuild Go bridge and create new release for v0.3.13

### Problem: timeout saga (chronological)

1. **Original**: `max_runtime_minutes=330` with `timeout-minutes:300` on step. GH kills
   at 300 min before context fires at 330 → no partial results.

2. **Fix #1**: `max_runtime→290`, deeper buffer. But step still `timeout-minutes:300`.
   Context fires at 290 min, but post-processing of 1M+ jobs takes >10 min.
   GH kills at 300 before post-processing finishes.

3. **Fix #2**: `max_runtime→250`, even more buffer. Step timeout still 300.
   Better, but still limited by step timeout.

4. **Fix #3**: Removed step-level `timeout-minutes:300` entirely. Added job-level
   `timeout-minutes:360`. `max_runtime→330`. But scrappy doesn't properly respect
   context cancellation — runs went 11h+ because scrapers ignore cancelled context.

5. **Fix #4 (065c312)**: Job-level `timeout-minutes:360` is the real hard cap.
   `max_runtime→300` (5h). 60 min buffer for post-processing, run_log, Telegram.
   GH hard-kills at 360 min (6h) no matter what.

### scrappy v0.3.13
- Added `ctx.Err() != nil` check in main processing loop (engine.go:671)
  → returns partial results immediately when context expires
- Go bridge locally updated (gitignored — needs release rebuild)
