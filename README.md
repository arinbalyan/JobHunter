<div align="center">

# JobHunter

**An open-source AI-powered job outreach agent.**
Scrape 55+ job boards, match jobs to your profile, send personalized emails with open/click tracking.

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Attribution-green)](https://github.com/arinbalyan/JobHunter)
[![Tests](https://img.shields.io/github/actions/workflow/status/arinbalyan/JobHunter/tests.yml?label=Tests&logo=github)](https://github.com/arinbalyan/JobHunter/actions/workflows/tests.yml)
[![Scrape](https://img.shields.io/github/actions/workflow/status/arinbalyan/JobHunter/scrape.yml?label=Scrape&logo=github)](https://github.com/arinbalyan/JobHunter/actions/workflows/scrape.yml)
[![Send](https://img.shields.io/github/actions/workflow/status/arinbalyan/JobHunter/send.yml?label=Send&logo=github)](https://github.com/arinbalyan/JobHunter/actions/workflows/send.yml)
[![Follow-up](https://img.shields.io/github/actions/workflow/status/arinbalyan/JobHunter/followup.yml?label=Follow-up&logo=github)](https://github.com/arinbalyan/JobHunter/actions/workflows/followup.yml)
[![Cleanup](https://img.shields.io/github/actions/workflow/status/arinbalyan/JobHunter/cleanup.yml?label=Cleanup&logo=github)](https://github.com/arinbalyan/JobHunter/actions/workflows/cleanup.yml)

</div>

---

## What is JobHunter?

JobHunter is a **Go-based** automated job scraping and email outreach system. It:

1. **Scrapes** job boards via the [scrappy](https://github.com/arinbalyan/scrappy) engine (55+ boards)
2. **Filters** jobs using configurable title rejection patterns, email filters, and deduplication
3. **Generates** personalized outreach emails using LLMs (10+ providers, round-robin)
4. **Sends** emails via Gmail SMTP with tracking pixels and optional resume attachments
5. **Tracks** opens, clicks, bounces, and replies via a built-in tracking server + IMAP scanner
6. **Follows up** automatically on sent emails that haven't received replies
7. **Reports** results to Telegram after every run

## 11 Commands

| Command | Purpose |
|---------|---------|
| `scrape` | Run scrappy, filter jobs, insert pending items into queue |
| `send` | Generate LLM emails and send pending queued items |
| `followup` | Find sent+no-reply emails and queue gentle follow-ups |
| `cleanup` | Delete old skipped jobs and mark stale pending items |
| `tracker` | Standalone HTTP server for open/click tracking |
| `inbox` | Show telemetry dashboard: engagement rates, IMAP scan, daily stats |
| `doctor` | 15-point diagnostic checklist (auto-creates `.env`) |
| `migrate` | Manual database migrations (up/drop) |
| `botid` | Discover your Telegram chat ID |
| `syncsecrets` | Push `.env` values to GitHub Secrets via `gh` CLI |
| `send` (legacy alias: `sender`) | The main email-sending workflow |

## Quick Start

```bash
# Clone
git clone https://github.com/arinbalyan/JobHunter.git
cd JobHunter

# Start local Postgres + tracker
docker compose up -d

# Run doctor (auto-creates .env if missing)
go run ./cmd/doctor/

# Edit .env with your API keys
# Then scrape jobs
go run ./cmd/scrape/

# Send pending emails (dry run first)
go run ./cmd/send/ --dry-run

# Check telemetry
go run ./cmd/inbox/
```

## Prerequisites

- **Go 1.26+**
- **PostgreSQL** (NeonDB free tier recommended, or use Docker Compose for local dev)
- **Gmail account** with [App Password](https://myaccount.google.com/apppasswords)
- **OpenRouter API key** (free, for LLM email generation) or any of 10+ supported providers
- **scrappy** (optional -- the scrape workflow builds it from source in CI, or install locally from [github.com/arinbalyan/scrappy](https://github.com/arinbalyan/scrappy))

## Architecture

```
cmd/                          internal/
├── scrape/                   ├── config/        env + YAML config + model discovery
├── send/                     ├── db/            PostgreSQL pool, queries, migrations
├── followup/                 ├── dedup/         email dedup, title rejection, email filters
├── cleanup/                  ├── scraper/       scrappy CLI adapter
├── tracker/                  ├── email/
├── migrate/                  │   ├── sender/    SMTP with retry + resume attachment
├── inbox/                    │   ├── tracker/   HTTP server (/track, /click, /health, /stats, /version)
├── doctor/                   │   └── imap/      IMAP scanner for bounces/replies
├── botid/                    ├── llm/
├── syncsecrets/              │   ├── router/    Multi-provider round-robin dispatcher
plugins/                      │   ├── providers/ llm.yaml loader (10 providers)
├── jobhunter.go              │   └── prompt/    System + user prompt builders
└── register.go               ├── logging/       Structured logger
                              ├── migrations/    5 SQL migration files
                              ├── template/      HTML email templates (embedded)
                              ├── stats/         Time-series stats collector
                              ├── telegram/      Telegram bot notifications
                              ├── report/        Run report generation
                              ├── job/           Job filtering and matching
                              └── plugin/        Plugin SDK + manager
```

## LLM Providers (10+)

The router dynamically discovers free OpenRouter models at startup and load-balances across all configured providers:

| Provider | Complex Models | Simple Models |
|----------|---------------|---------------|
| OpenRouter | gemma-4-26b:free (auto-discovers 28+) | openrouter/free |
| Groq | llama-3.3-70b | llama-3.1-8b |
| Together | llama-3.3-70b-turbo | gemma-4-9b |
| DeepInfra | llama-3.3-70b | llama-3.3-70b |
| Fireworks | llama-v3p3-70b | llama-v3p3-70b |
| Hyperbolic | llama-3.3-70b | llama-3.3-70b |
| SambaNova | Meta-Llama-3.3-70B | Meta-Llama-3.3-70B |
| Cerebras | gemma-4-9b | gemma-4-9b |
| NVIDIA | nemotron-4-340b | nemotron-4-340b |
| Z.AI | GLM-4-Plus | GLM-4-Air |

## Documentation

| # | Guide | Description |
|---|-------|-------------|
| 001 | [Getting Started](docs/guides/001-Getting-Started.md) | Setup, configuration, first run |
| 002 | [Plugin System](docs/guides/002-Plugin-System.md) | Writing and registering plugins |
| 003 | [Email Tracking](docs/guides/003-Email-Tracking.md) | Open/click tracking, IMAP bounce/reply detection |
| 004 | [LLM Router](docs/guides/004-LLM-Router.md) | Multi-provider AI routing, model discovery |
| 005 | [Database Schema](docs/guides/005-Database-Schema.md) | All tables, migrations, indexes |
| 006 | [GitHub Actions](docs/guides/006-GitHub-Actions.md) | CI/CD, secrets, automation |
| 007 | [Commands Reference](docs/guides/007-Commands-Reference.md) | All 11 CLI commands in detail |
| 008 | [Docker Deployment](docs/guides/008-Docker-Deployment.md) | Local dev with Docker Compose |
| 009 | [Telemetry & Monitoring](docs/guides/009-Telemetry-and-Monitoring.md) | Tracking server, inbox, IMAP |
| 010 | [Troubleshooting](docs/guides/010-Troubleshooting.md) | Common issues and fixes |

## Docker

```bash
# Start local Postgres and tracking server
docker compose up -d

# View tracking logs
docker compose logs -f tracker
```

## GitHub Actions

The project runs fully from GitHub Actions — no server needed. Six workflows automate the pipeline:

| Workflow | Schedule | Status |
|----------|----------|--------|
| Scrape Jobs | 4x daily (6/12/18/24 IST) | Active |
| Send Emails | Daily 8 AM IST | Active |
| Follow-up | -- | Disabled -- needs review |
| Bounce Scan | Daily 10 AM IST | Active |
| Cleanup | Weekly Sun 10 PM IST | Active |
| Deploy Tracker | On push to main | Active |

## Project Status

**Phase: Early Development.** Active development on `dev` branch. `main` still contains old Python V2 code. Not yet ready for production or merge to main.

## TODO

- [ ] **Re-enable follow-up workflow** — `.github/workflows/followup.yml` has its cron schedule disabled. The follow-up logic (domain-deduped follow-ups for sent+no-reply emails) is implemented but needs review and testing before re-enabling. Uncomment the `cron` line in the workflow file to re-enable.
- [ ] **Re-enable LLM email generation** — Currently running in `--fallback-only` mode with Go template-based emails. Uncomment LLM sections and remove the `--fallback-only` flag from `send-emails.yml` to use multi-provider LLM routing.
- [ ] **ATS scraper seeds** — ATS scrapers (workday, greenhouse, ashby, etc.) need `SCRAPPY_{PROVIDER}_SEEDS` environment variables with company slugs. Add to GitHub Secrets + `.env` once configured.
- [ ] **Resume PDF path** — Verify `.agent-data/ArinBalyan.pdf` is accessible in CI. The resume attachment is sent with every email when configured.

## License

Attribution-required open source. Credit the original repository and author when using, distributing, or building upon this work.

## Credits

- [scrappy](https://github.com/arinbalyan/scrappy) -- Fast Go-based job scraper with 55+ boards
- [golang-migrate](https://github.com/golang-migrate/migrate) -- Database migrations
- [pgx](https://github.com/jackc/pgx) -- PostgreSQL driver
