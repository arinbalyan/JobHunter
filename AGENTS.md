# Agent Configuration

## Project Status
**Phase: Rust Rewrite — 2026-06-26 Decision.**
The Go version on `dev` is the current running code. A Rust rewrite is planned based on lessons from the Go architecture. The Go `dev` branch remains active until the Rust version reaches feature parity.

## Language Stack
- **Rust** — primary application binary (`jobhunter`). All business logic, LLM router, email sending, tracking, Telegram alerts.
- **Go** — thin ~100-line bridge binary (`scraper`) wrapping `scrappy` as a direct Go library import. Called as a subprocess by the Rust binary. Reads config, calls `scrappy.ScrapeJobs()`, writes JSON to stdout. No business logic.

```
Released as a tarball containing both binaries.
User runs:  ./jobhunter scrape
                │
                └── spawns → ./scraper --config ...  (Go, invisible to user)
                                │
                                └── JSON stdout → serde Deserialize in Rust
```

## Database
- **Postgres via NeonDB** — cloud, persistent across GitHub Actions runs. SQLite is not viable: GH Actions runners are ephemeral, all data would vanish per-run.
- No Docker, no local Postgres, no migration tooling.
- `sqlx::migrate!()` — SQL embedded in the Rust binary, auto-run on first connect.
- One connection string in config.toml, that's it.

## Config
**Single file: `.data/config.toml`**

Holds everything non-secret:
- User profile
- Search terms, locations, sites
- LLM provider API keys (inlined or `$VAR` references resolved at load)
- Provider models, weights, base URLs
- Email limits, delays, dedup windows
- All prompt templates (system, user, follow-up, scoring, research, triage)
- Resume paths, Telegram chat IDs

API keys come from env vars or `$VAR` references in the config. No separate `.env`. No separate `llm.yaml`. No `MergeIntoConfig()` bridge.

## LLM Router
Same 9 providers as Go version. Same weighted round-robin + failover chain. Written in Rust using `reqwest` + `serde_json`. Expanded use cases:

| Feature | Complexity | Status |
|---|---|---|
| Email generation | Complex | ✅ Current Go feature |
| Job scoring (1-10 match) | Simple | ➕ New |
| Follow-up generation | Complex | ➕ Enhanced (was hardcoded template) |
| Company research (3 talking points) | Medium | ➕ New |
| Gap analysis (framing strategy) | Complex | ➕ New |
| Reply triage (positive/negative/neutral) | Simple | ➕ New |

Health tracking: 3 consecutive failures → 30s cooldown, auto-recovery. Same as Go, fewer lines via `tower` middleware.

## Pipeline
Concurrent by default:
- **Scrape**: scrappy called once per config (it handles site parallelism internally).
- **LLM gen**: N goroutines (tokio tasks), bounded by semaphore (default 10 concurrent).
- **Send**: token bucket rate limiter (default 1 per 15s), fire-and-forget per email.
- **Follow-up + cleanup**: same pattern, simpler logic.

## What Changes From Go Version

| Aspect | Go (current) | Rust (planned) |
|---|---|---|
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

## What Stays The Same
- 4-workflow pipeline: scrape → send → followup → cleanup
- Tracker HTTP server (same routes, same pixel, same DB updates)
- Telegram alerts per run
- Email tracking via 1x1 pixel + click redirect
- Gmail SMTP (587, STARTTLS, app password)
- Dedup: atomic SQL `INSERT ... WHERE NOT EXISTS`, cooldown windows
- MX verification before sending

## LLM Providers (9, all OpenAI-compatible)

| Provider | Complex | Simple | Notes |
|----------|---------|--------|-------|
| OpenRouter | gemma-4-26b:free | openrouter/free | Auto-discovers 28+ free models |
| Groq | llama-3.3-70b | llama-3.1-8b | Very fast inference |
| Together | llama-3.3-70b-turbo | gemma-4-9b | — |
| DeepInfra | llama-3.3-70b | llama-3.3-70b | — |
| Fireworks | llama-v3p3-70b | llama-v3p3-70b | — |
| Hyperbolic | llama-3.3-70b | llama-3.3-70b | — |
| SambaNova | Meta-Llama-3.3-70B | Meta-Llama-3.3-70B | — |
| Cerebras | gemma-4-9b | gemma-4-9b | Wafer-scale fast |
| Z.AI | GLM-4-Plus | GLM-4-Air | Chinese LLM |

Router uses weighted round-robin + failover chain up to 3 providers. Health tracking marks providers unhealthy after 3 consecutive failures, auto-recovers after 30s cooldown.

## Release Model
- Single tarball: `jobhunter-v1.0.0-x86_64-linux.tar.gz` containing both binaries
- Install: `curl ... | tar xz -C /usr/local/bin`
- Config: user writes `.data/config.toml`
- Run: `./jobhunter scrape`
- No Docker, no npm, no pip, no runtime deps
