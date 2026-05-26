# Agent Configuration

## Project Status
**Phase: Early Development -- Not ready for production/main merge.**
We are in the initial scaffolding and feature-building phase.
The `dev` branch is active; `main` keeps the old Python V2 code for history.

**MERGE POLICY:** Do NOT merge `dev` into `main` without explicit user permission.
When permission is given, verify THREE times with the user before executing.
The merge sequence will be:
1. First commit on `main` that removes all Python V2 files (`src/` directory)
2. Then merge `dev` into `main` (no conflicts since Python files are deleted first)
3. Tag v1.0.0 release

## Project Overview
Go port of JobHunter -- a job scraping and email automation system.
Uses the Go-based scrappy CLI (`/home/nemesis/projects/scrappy`) instead of Python jobspy.

## Agent Personality
- **Direct**: Concise, no fluff
- **Context-aware**: Uses RULES.md, CONTEXT.md, config.yaml, and llm.yaml for decisions
- **Honest**: Will point out when job requirements exceed experience level
- **Strategic**: Helps reframe underqualified applications effectively

## Core Capabilities
- Job scraping via scrappy CLI (55+ boards, non-interactive mode)
- Job filtering: title rejection, email filtering, dedup (per-email/domain/company cooldowns)
- LLM-powered email generation (10+ providers, weighted round-robin)
- Gmail SMTP sending with tracking pixel, 30s delay, optional resume attachment
- Email tracking: open/click pixels, IMAP bounce/reply detection
- Telegram alerts for all workflow runs (scrape, send, followup, cleanup)
- 4-workflow pipeline: scrape -> send -> followup -> cleanup

## Architecture (Current)

```
cmd/                       ✦ 11 CLI commands
├── scrape/     → scrappy CLI → DB (pending/skipped jobs with reasons)
├── send/       → email_queue → SMTP (with LLM generation via router)
├── followup/   → sent+no_reply → queue follow-up items
├── cleanup/    → delete old skipped + mark stale pending
├── tracker/    → HTTP server (/track, /click, /health, /stats, /version)
├── migrate/    → manual DB migrations (up/drop)
├── doctor/     → 15-point diagnostic checklist (auto-creates .env)
├── inbox/      → engagement telemetry + IMAP scan dashboard
├── botid/      → discover Telegram chat ID
└── syncsecrets/→ push .env to GitHub Secrets via gh CLI

internal/        ✦ Core packages
├── config/     → env loader + YAML config (.agent-data/config.yaml) + model discovery
├── db/         → PostgreSQL pool, queries, migrations, run_logs, workflow helpers
├── dedup/      → email dedup (cooldown), title rejection, email filtering
├── email/
│   ├── sender/ → Gmail SMTP with retry, MIME building, resume attachment
│   ├── tracker/→ HTTP server with live atomic counters
│   └── imap/   → IMAP scanner for bounces/replies
├── llm/
│   ├── router/ → Multi-provider weighted round-robin dispatcher
│   ├── providers/ → llm.yaml loader (10 provider kinds)
│   └── prompt/ → System + user prompt builders
├── logging/    → Structured logger with levels
├── migrations/ → 5 SQL migration files (000001-000005)
├── template/   → Embedded HTML email templates (gohtml)
├── scraper/    → scrappy CLI adapter (--config, --non-interactive)
├── stats/      → Time-series stats collector
├── telegram/   → Telegram bot notification client
├── report/     → Run report generation
├── job/        → Job matching utilities
└── plugin/     → Plugin SDK + manager (for future extensibility)

plugins/         ✦ Built-in plugins
├── jobhunter.go → Core job outreach plugin
└── register.go  → Plugin registration

.agent-data/     ✦ Agent configuration (user-facing)
├── config.yaml → Search terms, filters, limits, user profile
├── llm.yaml    → LLM provider -> model mapping (optional, env fallback)
├── CONTEXT.md  → User context for LLM prompts (gitignored)
└── RULES.md    → Behavioral rules (gitignored)
```

## Key Design Decisions
- **Plugin architecture**: Everything is a plugin via `internal/plugin/sdk` (extensibility, not mandatory)
- **CLI-first**: scrappy runs as a subprocess, not a library import
- **4-workflow pipeline**: scrape inserts jobs -> send picks up pending -> followup queues follow-ups -> cleanup removes old
- **LLM provider config**: Separate YAML file per provider with task-specific models (optional -- env-only works too)
- **No word limits on emails**: LLM decides length naturally (configurable min/max bounds)
- **Run logs**: Every workflow run is recorded with metrics in the database
- **Telegram-native**: All workflows send formatted HTML reports on completion

## LLM Providers (10+)
| Provider | Complex | Simple | Reasoning | Notes |
|----------|---------|--------|-----------|-------|
| OpenRouter | gemma-4-26b:free | openrouter/free | deepseek-v4:free | Auto-discovers 28+ free models |
| Groq | llama-3.3-70b | llama-3.1-8b | -- | Very fast inference |
| Together | llama-3.3-70b-turbo | gemma-4-9b | -- | -- |
| DeepInfra | llama-3.3-70b | llama-3.3-70b | -- | -- |
| Fireworks | llama-v3p3-70b | llama-v3p3-70b | -- | -- |
| Hyperbolic | llama-3.3-70b | llama-3.3-70b | -- | -- |
| SambaNova | Meta-Llama-3.3-70B | Meta-Llama-3.3-70B | -- | -- |
| Cerebras | gemma-4-9b | gemma-4-9b | -- | Wafer-scale fast |
| NVIDIA | nemotron-4-340b | nemotron-4-340b | -- | -- |
| Z.AI | GLM-4-Plus | GLM-4-Air | -- | Chinese LLM |

All use OpenAI-compatible API format. Router uses weighted round-robin with fallback chain.

## Local Development
```bash
# Start local Postgres + tracker
docker compose up -d

# Run doctor (auto-creates .env if missing)
go run ./cmd/doctor/

# Scrape jobs
go run ./cmd/scrape/

# Send pending emails (dry run)
go run ./cmd/send/ --dry-run

# View telemetry
go run ./cmd/inbox/

# Run diagnostics
go run ./cmd/doctor/
```

## GitHub Actions Workflows

| Workflow | Schedule | Purpose |
|----------|----------|---------|
| `Scrape Jobs` | 4x daily (6/12/18/24 IST) | Build scrappy, scrape boards, filter jobs |
| `Send Emails` | Daily 8 AM IST | Generate LLM emails, send pending |
| `Follow-up` | Daily 9 AM IST | Queue follow-ups for sent+no-reply |
| `Cleanup` | Weekly Sunday 10 PM IST | Remove old skipped/stale jobs |
| `Deploy Tracking` | On push to main | Deploy tracker to Vercel |
| `Tests` | On push/PR | Run tests, vet, build |

## Integration with Scrappy
Scrappy is at `/home/nemesis/projects/scrappy` (separate repo).
JobHunter calls it via CLI subprocess with `--non-interactive --config <path>`.
In CI, the scrape workflow builds scrappy from source. Locally, it searches common paths.
Currently scrappy is also in early development. Both projects will mature together.

## What's Left Before Main Merge
- [ ] Scrappy stabilizes (its own dev cycle)
- [ ] Full end-to-end test with real email send
- [ ] Merge blocked: wait for user permission (verify 3x before executing)
- [ ] Tag v1.0.0 release
