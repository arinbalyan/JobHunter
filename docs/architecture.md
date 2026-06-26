# Architecture

```
User runs:  ./jobhunter scrape --mode remote
                │
                └── spawns → ./scraper (Go bridge, ~30 lines)
                                │
                                └── stdin JSON (search params + timeout)
                                    → scrappy.NewEngine().ScrapeJobs()
                                    → stdout JSON ([]JobPost)
```

## Two binaries, one user-facing

| Binary | Language | Lines | What it does |
|--------|----------|-------|-------------|
| `jobhunter` | Rust | ~700 | Business logic: CLI, config, filters, DB, LLM, email |
| `scraper` | Go | ~30 | Bridge: reads JSON from stdin, calls scrappy, writes JSON to stdout |

The Rust binary is the only one the user interacts with. It spawns `./scraper` as a subprocess, sends search params via stdin, reads job results from stdout. The Go bridge has zero business logic — it's a pass-through to `scrappy`.

## scrappy handles the hard part

[scrappy](https://github.com/arinbalyan/scrappy) is the scraping engine. It handles everything scraping-related:

- 141 sites, 49 working out of the box
- 28 ATS providers with 2,291 embedded company slugs
- Per-site rate limiting (token bucket + global semaphore)
- Email extraction pipeline (HTML mailto: → regex deobfuscation → MX verify)
- Quality scoring (0-100, deterministic)
- URL dedup, memory management, proxy support

JobHunter **trusts scrappy's verdict**. It doesn't re-check MX, doesn't re-score. It only:
- Filters rejected titles (~40 patterns)
- Strips obvious junk emails (no-reply@, do-not-reply@, suspicious TLDs)
- De-duplicates across runs via SQL

## Pipeline

```
┌─ Scrape ──────────────────────────────────────────────┐
│  CLI → config → BridgeInput JSON                      │
│  → spawn ./scraper → read stdout → Vec<JobPost>       │
│  → title filter → email filter                        │
│  → INSERT jobs WHERE NOT EXISTS (dedup by job_url)    │
│  → INSERT email_queue (dedup by addr+company, 30d)    │
│  → Telegram report                                     │
│  → Pending job carry-over from prev runs               │
└───────────────────────────────────────────────────────┘

┌─ Send ────────────────────────────────────────────────┐
│  CLI → query email_queue WHERE status = 'pending'     │
│  → for each (up to 10 concurrent):                    │
│      build prompt → Router::complete()                 │
│      → parse SUBJECT:/body → UPDATE email_queue        │
│      → on failure: template fallback                   │
└───────────────────────────────────────────────────────┘
```

## LLM Router

9 providers, all OpenAI-compatible. Weighted random selection:

| Provider | Weight | Complex Model | Simple Model |
|----------|--------|---------------|--------------|
| OpenRouter | 10 | google/gemma-4-31b-it:free | openrouter/free |
| Groq | 5 | openai/gpt-oss-120b | openai/gpt-oss-20b |
| Together | 4 | google/gemma-4-9b-it | same |
| DeepInfra | 4 | meta-llama/Llama-3.3-70B-Instruct-Turbo | same |
| Hyperbolic | 3 | qwen3-coder-480b-a35b-instruct:free | deepseek-r1-0528:free |
| SambaNova | 3 | Meta-Llama-3.3-70B-Instruct | same |
| Cerebras | 2 | zai-glm-4.7 | gpt-oss-120b |
| Z.AI | 1 | GLM-4-Plus | GLM-4-Air |

**Failover**: if a provider returns an error after 3 consecutive failures, it's cooled down for 30s. Up to 3 providers are tried per request before falling back to template.

**Model discovery**: on startup, each provider's `/models` endpoint is checked. Warnings are logged if configured models aren't found.

## Database

Postgres via [NeonDB](https://neon.tech) (direct wire protocol). 4 tables:

| Table | Purpose |
|-------|---------|
| `run_log` | Per-run stats (workflow, status, counts) |
| `jobs` | Deduped job posts from scrapes |
| `email_queue` | Emails to send (pending → generating → generated → sent/failed) |
| `tracking` | Open/click/reply tracking |

Migrations are embedded in the binary via `sqlx::migrate!()` and auto-run on first connect.

## Config

Single file: `.data/config.toml` (or `config.toml` in project root). API keys referenced as `$VAR_NAME` resolved from environment at load time. See `docs/configuration.md` for full reference.
