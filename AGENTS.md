# Agent Configuration

## Project Status
**Phase: Rust Rewrite — 2026-06-26 Decision.**
The Go version on `dev` is the current running code. A Rust rewrite is planned based on lessons from the Go architecture. The Go `dev` branch remains active until the Rust version reaches feature parity.

**IMPORTANT**: When a phase is completed, update the `## Roadmap` section below by marking that phase `✅ Done` and record the completion date. Keep AGENTS.md as the single source of truth for project state.

## Language Stack
- **Rust** — primary application binary (`jobhunter`). All business logic, LLM router, email sending, tracking, Telegram alerts.
- **Go** — thin ~100-line bridge binary (`scraper`) wrapping `scrappy` as a direct Go library import. Called as a subprocess by the Rust binary. Reads JSON from stdin, calls `scrappy.ScrapeJobs()`, writes JSON to stdout. No business logic.

```
Released as a tarball containing both binaries.
User runs:  ./jobhunter scrape
                │
                └── spawns → ./scraper (Go bridge, invisible to user)
                                │
                                └── stdin JSON (ScraperInput)
                                    → scrappy.NewEngine().ScrapeJobs()
                                    → stdout JSON ([]JobPost)
```

## Database
- **Postgres via NeonDB (direct wire protocol)** — Neon is standard Postgres wire protocol (`postgresql://user:pass@host.neon.tech/db?sslmode=require`). No HTTP proxy, no API layer. The `sqlx` crate connects straight to the Postgres backend with `libpq` wire protocol, same as any other Postgres. Zero overhead.
- Cloud, persistent across GitHub Actions runs. SQLite not viable: GH Actions runners are ephemeral, all data would vanish per-run.
- No Docker, no local Postgres, no migration tooling.
- `sqlx::migrate!()` — SQL embedded in the Rust binary, auto-run on first connect.
- One connection string in config.toml, that's it.

## Config
**Single file: `.data/config.toml`**

Holds everything non-secret:
- User profile
- Search terms, locations, sites (with separate onsite/remote presets)
- LLM provider API keys (inlined or `$VAR` references resolved at load)
- Provider models, weights, base URLs
- Email limits, delays, dedup windows
- All prompt templates (system, user, scoring, research, triage)
- Resume paths, Telegram chat IDs

API keys come from env vars or `$VAR` references in the config. No separate `.env`. No separate `llm.yaml`. No `MergeIntoConfig()` bridge.

## LLM Router
Same 9 providers as Go version. Same weighted round-robin + failover chain. Written in Rust using `reqwest` + `serde_json`. 

**Free models only** — the router exists specifically to pool multiple free-tier API keys so you never hit rate limits or pay per-token. Using latest free models researched June 2026:

| Provider | Complex Model | Simple Model | Free Tier Notes |
|----------|--------------|--------------|-----------------|
| OpenRouter | google/gemma-4-31b-it:free | openrouter/free | 20+ free models, auto-routes best available |
| Groq | openai/gpt-oss-120b | openai/gpt-oss-20b | Llama-3.x deprecated June 17 2026. 30 RPM free. |
| Cerebras | zai-glm-4.7 | gpt-oss-120b | Wafer-scale fast. Free tier confirmed. |
| Together | gemma-4-9b-it | gemma-4-9b-it | Several free models available |
| DeepInfra | meta-llama/Llama-3.3-70B-Instruct | meta-llama/Llama-3.3-70B-Instruct | Free tier available |
| Hyperbolic | qwen3-coder-480b:free | deepseek-r1-0528:free | Free serverless models |
| SambaNova | Meta-Llama-3.3-70B-Instruct | Meta-Llama-3.3-70B-Instruct | $5 free credits on signup |
| Fireworks | llama-v3p3-70b-instruct | llama-v3p3-70b-instruct | No ongoing free tier (only $1 signup credit). Drop if no key. |
| Z.AI | GLM-4-Plus | GLM-4-Air | Chinese LLM, free tier unclear |

Health tracking: 3 consecutive failures → 30s cooldown, auto-recovery. Same as Go, fewer lines via `tower` middleware.

## LLM Features (Minimal — Free Models Only)

| Feature | Complexity | Cost Concern | Status |
|---------|-----------|--------------|--------|
| **Email generation** | Complex | One call per email. Templates as fallback. | ✅ Keep |
| **Job scoring (1-10)** | Simple | One short call per job. Triage step. | ➕ New |
| **Company research (3 talking points)** | Medium | One call per company. Optional enrichment. | ➕ New |
| **Reply triage (positive/negative/neutral)** | Simple | One call per reply. Auto-classify. | ➕ New |

**NOT building**: follow-up generation, gap analysis, multi-turn conversation. These burn tokens for marginal value. The router + free providers is to keep costs at $0, not to scale up LLM usage.

## Pipeline
Concurrent by default. scrappy handles parallelism internally — Rust just reads the output JSON.

```
Rust: build ScraperInput from config → spawn ./scraper with stdin JSON
  │
  ▼ Go bridge: engine.ScrapeJobs(ctx, input)
    ├── fan-out: 141 sites concurrently (goroutine pool)
    ├── per-scraper: email extraction → MX verify → quality score
    ├── in-flight dedup by URL/ID
    └── results → stdout JSON array
  │
  ▼ Rust: deserialize → title filter → gentle email filter → SQL dedup → email_queue
```

- **Scrape**: Rust builds ScraperInput JSON from config, spawns Go subprocess, reads stdout. scrappy handles all concurrency, rate limiting, email extraction, MX verification, and quality scoring.
- **LLM gen**: N tokio tasks, bounded by semaphore (default 10 concurrent).
- **Send**: token bucket rate limiter (default 1 per 15s), fire-and-forget per email.
- **Follow-up + cleanup**: disabled. Not needed with free-tier-only LLM strategy.

## scrappy Integration Architecture

scrappy is a standalone Go module at `~/projects/scrappy/` used as a **direct Go library import** (`github.com/arinbalyan/scrappy/pkg/scrappy`). The Rust binary calls it through a thin Go subprocess bridge.

**📝 scrappy improvements**: Whenever you find a limitation or something scrappy could do better, document it in `scrappy_improvements.md` (gitignored). This file is the backlog for scrappy's next version. Check it before reporting bugs to avoid duplicates.

### Why a Go subprocess?
Rust can't link against Go libraries. The bridge is ~100 lines of Go: read stdin JSON, call scrappy, write stdout JSON. That's it — no business logic, no config parsing, no filtering.

### Interface
```
Rust: echo 'ScraperInput JSON' | ./scraper
Go:   engine := scrappy.NewEngine()
      jobs, err := engine.ScrapeJobs(ctx, input)
      json.NewEncoder(os.Stdout).Encode(jobs)
Rust: serde_json::from_reader::<Vec<JobPost>>(stdout)
```

### What scrappy handles (Rust never reimplements)
| Feature | Details |
|---------|---------|
| **Site parallelism** | 141 sites, fan-out per site with goroutine pool |
| **Per-site rate limiting** | Token bucket + global semaphore, configurable RPS |
| **Email extraction** | 4-stage pipeline: HTML mailto: → regex deobfuscation → company page enrichment → MX DNS verify |
| **MX verification** | DNS MX lookup per email, optional SMTP RCPT TO check. `VerifyConcurrency` controls parallelism. |
| **Quality scoring** | Deterministic 0-100: salary(20) + direct-apply(15) + email-domain-match(15) + freshness(15) + verified-email(10) + description-length(10) + not-agency(10) + multiple-emails(5) |
| **URL dedup** | In-flight dedup by JobURL/ID to save memory |
| **Auto-discovery** | ATS scrapers find companies via embedded slug file (2,291 slugs across 28 providers) |
| **Proxy support** | SOCKS5/HTTP health-checked round-robin |
| **Memory management** | Configurable cap with auto-GC + concurrency scaling |
| **Browser fallback** | Optional Playwright for LinkedIn, Monster, Google |

### What Rust handles (business logic)
| Feature | Notes |
|---------|-------|
| **Title rejection** | ~40 patterns to skip irrelevant jobs |
| **Gentle email filter** | Only strip obvious junk: no-reply@, do-not-reply@, suspicious TLDs. Trust scrappy's MX verdict. |
| **SQL atomic dedup** | `INSERT ... WHERE NOT EXISTS` across runs by email/domain/company + cooldown windows |
| **LLM email generation** | Personalized outreach via 9 free providers |
| **LLM job scoring** | 1-10 match score per job |
| **LLM company research** | 3 talking points per company |
| **LLM reply triage** | Positive/negative/neutral classification |
| **SMTP sending** | Gmail 587 STARTTLS with rate limiting and quota detection |
| **Email tracking** | 1x1 pixel open + click redirect |
| **Telegram alerts** | Per-workflow HTML reports |
| **Run log persistence** | Per-run stats in Postgres |
| **Pending job carry-over** | Unprocessed jobs from previous runs get re-queued |

### Config approach
scrappy has its own `config.toml` with per-site optimized search terms for all 141 sites. JobHunter does NOT need to duplicate that — the bridge passes high-level `SearchTerms`/`Locations`/`Sites` from JobHunter's config, and scrappy's engine uses those as global fallbacks when per-site overrides are empty.

If you want scrappy's per-site search optimizations, the bridge can be extended to load scrappy's config.toml — but for now, the global terms work fine for the ~49 working sites.

### Current scrappy status
Latest: **v0.3.5** — 141 sites
- 49 working out of the box
- 28 ATS providers with company slug discovery (566 Ashby slugs alone)
- 15 sites need API keys (keys in `.env` unlock them)
- 10 niche boards (RSS feeds, no SWE jobs)
- Rest are timeout/broken/stale slugs

See full breakdown: `~/projects/scrappy/documentation/status/`

### scrappy API Keys in JobHunter
| Env Var | Site | Status |
|---------|------|--------|
| `SCRAPPY_INDEED_API_KEY` | Indeed | ✅ In `.env` |
| `SCRAPPY_DICE_API_KEY` | Dice | ✅ In `.env` (added from scrappy) |

Optional scrappy keys (see `~/projects/scrappy/.env.example`): `ADZUNA_APP_ID`, `CAREERJET_AFFID`, `FINDWORK_API_KEY`, `WEB3CAREER_API_TOKEN`, etc.

## Onsite / Remote Modes
scrappy has no concept of "mode." Both are just different config presets passed to the same `ScraperInput`:

- **Remote mode**: remote sites, remote_only=true, global locations
- **Onsite mode**: India/region-specific sites, remote_only=false, local locations

Config.toml holds both sets of search params. The CLI `./jobhunter scrape --mode remote` or `--mode onsite` picks which to use. The Go scraper binary receives the same struct either way.

```
[search.remote]
terms = ["AI Engineer", "ML Engineer"]
locations = ["remote"]
sites = ["linkedin", "indeed", "greenhouse"]
remote_only = true

[search.onsite]
terms = ["software engineer", "backend developer"]
locations = ["bangalore", "mumbai"]
sites = ["linkedin", "indeed", "naukri"]
remote_only = false
```

## MX Verification (Filter Layer)
scrappy already does MX verification internally (via `VerifyEmail: true`). The Rust side should NOT re-check MX. Just use scrappy's `Verified` + `QualityScore` fields to prioritize, not filter. Goal: avoid filtering out real recruiter emails. Keep it gentle — trust scrappy's verdict, only filter obvious junk (no-reply@, do-not-reply@, suspicious TLDs).

## What Changes From Go Version

| Aspect | Go (current) | Rust (planned) |
|--------|-------------|----------------|
| Language | Go + scrappy direct import | Rust + Go scraper subprocess |
| Database | Postgres + pgx/v5 + golang-migrate | Postgres + sqlx + embedded migrations |
| Config | 4 files (env, config.yaml, llm.yaml, MergeIntoConfig) | 1 file (config.toml) |
| Binaries | 10 separate cmd/* mains | 2 binaries (1 user-facing) |
| LLM usage | Email gen only | Email + scoring + research + triage |
| Pipeline | Sequential per-item | Concurrent gen + rate-limited send |
| Template flexibility | Hardcoded in prompt.go | In config.toml, user-editable |
| Concurrency model | sync.WaitGroup + atomic | tokio + tower middleware |
| Error handling | if err != nil × N | anyhow::Result + ? operator |
| JSON handling | Manual marshal/unmarshal | serde derive macros |
| MX verification | Duplicated (scrappy + Go net.LookupMX) | scrappy only, trust its verdict |

## What Stays The Same
- 4-workflow pipeline: scrape → send → followup → cleanup (but followup + cleanup disabled)
- Tracker HTTP server (same routes, same pixel, same DB updates)
- Telegram alerts per run
- Email tracking via 1x1 pixel + click redirect
- Gmail SMTP (587, STARTTLS, app password)
- Dedup: atomic SQL `INSERT ... WHERE NOT EXISTS`, cooldown windows
- scrappy v0.3.5 handles MX verification, quality scoring, email enrichment, URL dedup, per-site rate limiting — Rust trusts its output

## What's Dropped From Go Version
- **cmd/bouncescan/** (615 lines IMAP parsing — passive, Gmail notifies anyway)
- **cmd/botid/** (one-time setup, docs suffice)
- **cmd/syncsecrets/** (5 lines of shell)
- **internal/dedup/dedup.go** (pre-check duplicates SQL atomic gate)
- **internal/logging/** (use `tracing` crate)
- **internal/llm/providers/loader.go** + **internal/llm/prompt/** (templates go in config.toml)

## LLM Providers (9, all OpenAI-compatible)

See table above in LLM Router section. Models updated June 2026 to reflect deprecations and latest free tiers.

Router uses weighted round-robin + failover chain up to 3 providers. Health tracking marks providers unhealthy after 3 consecutive failures, auto-recovers after 30s cooldown.

---

## Release Model
- Single tarball: `jobhunter-v0.1.0-x86_64-linux.tar.gz` containing both binaries
- Install: `curl -fsSL https://github.com/arinbalyan/jobhunter/releases/latest/download/jobhunter-linux.tar.gz | tar xz -C /usr/local/bin`
- Config: user writes `.data/config.toml`
- Run: `./jobhunter scrape --mode remote`
- No Docker, no npm, no pip, no runtime deps

---

## Feature Inventory (from audit of Go dev + Python jobspy branches)

### Core Pipeline (Keep)
| Feature | Source | Priority |
|---------|--------|----------|
| scrappy scraper (100+ boards) | Go dev | P0 |
| Title rejection (~40 patterns) | Go dev | P0 |
| Email filtering (starts_with, contains, tld:) | Go dev | P0 |
| Atomic dedup (email/domain/company cooldown) | Go dev | P0 |
| MX verification (via scrappy, not re-checked) | Go dev | P0 |
| LLM email generation with router | Go dev | P0 |
| Fallback template emails (3 experience levels) | Go dev | P0 |
| Gmail SMTP with retry + quota detection | Go dev | P0 |
| Resume PDF attachment | Go dev | P0 |
| Email tracking pixel (open/click) | Go dev | P0 |
| Run log persistence | Go dev | P0 |
| Telegram alerts per workflow | Go dev | P0 |
| config.toml — single config file | New | P0 |

### New LLM Features (Build)
| Feature | Source | Priority |
|---------|--------|----------|
| Job scoring (1-10 match) | New | P1 |
| Company research (3 talking points) | New | P2 |
| Reply triage (positive/negative/neutral) | New | P2 |

### From Python Version (Port)
| Feature | Source | Priority |
|---------|--------|----------|
| Onsite/remote mode presets | Python | P1 |
| Pending job carry-over across runs | Python | P2 |
| Weekend skip option | Python | P3 |

### Dropped (Not Building)
| Feature | Source | Why |
|---------|--------|-----|
| IMAP bounce/reply scan | Go dev | Passive, Gmail notifies anyway, 615 lines |
| Google Sheets storage | Python | Postgres covers persistence |
| APScheduler cron | Python | GH Actions is the scheduler |
| Serve mode / health check | Python | Not needed for GH Actions |
| Keyword extraction | Python | Not used in pipeline |
| End-of-run email report | Python | Telegram covers this |
| botid (chat ID discovery) | Go dev | One-time setup, docs suffice |
| syncsecrets (gh CLI wrapper) | Go dev | 5 lines of shell |
| Follow-up generation | Go dev | Burns tokens, manual follow-up is fine |
| Gap analysis | Planned | Burns tokens for marginal value |

---

## Roadmap

### Phase 1: Scaffold + Config + DB (Current)
- [x] Rust project skeleton — Cargo.toml, clap subcommands
- [x] config.toml parsing — serde + toml + $VAR env resolution
- [x] Postgres connection — sqlx + embedded migrations
- [x] Initial schema — jobs, email_queue, tracking, run_log tables
- [x] Go scraper subprocess — stdin JSON → scrappy.ScrapeJobs() → stdout JSON
- [x] `./jobhunter doctor` — checks config, DB URL, scraper binary, LLM keys

### Phase 2: Scrape Workflow ✅
- [x] Rust serializes search params to JSON, spawns Go scraper
- [x] Deserialize scraper stdout → Vec<JobPost>
- [x] Title rejection filter (~40 patterns)
- [x] Gentle email filter (no-reply@, do-not-reply@, suspicious TLDs)
- [x] Atomic SQL dedup — INSERT WHERE NOT EXISTS by job_url
- [x] Email queue population per job
- [x] `results_wanted: 0` (unlimited) — scrappy returns all jobs it finds
- [x] `timeout_seconds` from config — bridge context uses `[scrape].max_runtime_minutes`
- [x] Pending job carry-over — INSERT-SELECT for unqueued jobs from last 7 days
- [x] Telegram report — scrape summary sent to chat after each run
- [x] GH Actions workflow — 4x daily, builds Rust + Go, runs scrape
- [x] Onsite/remote mode selector — `--mode remote|onsite`, two config presets with separate sites/terms/locations

### Phase 3: Send Workflow (Current)
- [x] LLM router — weighted random + failover chain (3 attempts)
- [x] Health tracking — 3 consecutive failures → 30s cooldown, auto-recovery
- [x] `/models` discovery — checks all 9 providers at startup, logs available models + warns if configured model missing
- [x] OpenAI-compatible completion — single `complete()` function works across all providers
- [x] Email generation via LLM (prompts from config.toml)
- [x] Template fallback emails — when all providers fail, uses a basic template
- [x] Concurrent generation — tokio + semaphore, default 10 concurrent
- [x] SMTP sender — lettre crate, Gmail 587 STARTTLS
- [ ] Resume PDF attachment (Phase 5)
- [x] Rate-limited send — token bucket (delay_seconds)
- [x] Tracking pixel injection — 1x1 img tag in HTML body
- [x] Quota tracking — daily sent count from DB, stops at daily_limit
- [ ] GH Actions workflow: send (daily)

### Phase 4: Tracker + Notifications
- [ ] HTTP tracking server (/track, /click, /health, /version)
- [ ] Open/click pixel logging to DB
- [ ] Telegram alerts per workflow
- [ ] Run log persistence

### Phase 5: Polish + Deploy
- [ ] LLM job scoring (1-10)
- [ ] LLM company research (3 talking points)
- [ ] LLM reply triage (positive/negative/neutral)
- [ ] `./jobhunter inbox` — telemetry dashboard
- [ ] GH Actions: scrape (4x daily) + send (daily)
- [ ] GH Actions: tests (on push/PR)
- [ ] Release packaging (tarball with both binaries)
- [ ] README with quick-start

### Phase 3: Send Workflow
- [ ] LLM router (9 providers, weighted round-robin, failover)
- [ ] Email generation via LLM (prompts from config.toml)
- [ ] Template fallback (for each experience match level)
- [ ] Concurrent generation (tokio + semaphore, max 10)
- [ ] SMTP sender (lettre crate, Gmail 587 STARTTLS)
- [ ] Resume PDF attachment
- [ ] Rate-limited send (token bucket, 1 per 15s)
- [ ] Tracking pixel injection
- [ ] Quota tracking (daily limit from config)
- [ ] Job scoring (1-10) — simple LLM classification
- [ ] Company research (3 talking points) — optional enrichment
- [ ] GH Actions workflow: send (daily)

### Phase 4: Tracker + Notifications
- [ ] HTTP tracking server (/track, /click, /health, /version)
- [ ] Both: standalone binary + Vercel serverless handler
- [ ] Open/click pixel logging to DB
- [ ] Telegram notifications for all workflows
- [ ] Run log persistence

### Phase 5: Polish + Deploy
- [ ] Reply triage (positive/negative/neutral via LLM)
- [ ] `./jobhunter inbox` — telemetry dashboard
- [ ] GH Actions: cleanup (weekly)
- [ ] GH Actions: tests (on push/PR)
- [ ] Release packaging (tarball with both binaries)
- [ ] config.example.toml (committed to repo)
- [ ] README with quick-start

---

## Completion Notes
When a phase is completed:
1. Mark it `✅ Done` and add the date in the Roadmap above
2. Create a git tag (e.g. `v0.1.0-phase1`)
3. Push to GitHub
4. Update this AGENTS.md with any architectural changes learned during the phase
5. Open a new todo list for the next phase
