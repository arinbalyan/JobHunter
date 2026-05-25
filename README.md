<div align="center">

# JobHunter

**An open-source AI-powered job outreach agent.**
Scrape job boards, match jobs to your profile, send personalized emails with tracking.

[![Go Version](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Attribution-green)](https://github.com/arinbalyan/JobHunter)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen)](CONTRIBUTING.md)
[![Tests](https://github.com/arinbalyan/JobHunter/actions/workflows/tests.yml/badge.svg)](https://github.com/arinbalyan/JobHunter/actions/workflows/tests.yml)
[![Scrape](https://github.com/arinbalyan/JobHunter/actions/workflows/scrape.yml/badge.svg)](https://github.com/arinbalyan/JobHunter/actions/workflows/scrape.yml)
[![Send](https://github.com/arinbalyan/JobHunter/actions/workflows/send.yml/badge.svg)](https://github.com/arinbalyan/JobHunter/actions/workflows/send.yml)
[![Follow-up](https://github.com/arinbalyan/JobHunter/actions/workflows/followup.yml/badge.svg)](https://github.com/arinbalyan/JobHunter/actions/workflows/followup.yml)
[![Cleanup](https://github.com/arinbalyan/JobHunter/actions/workflows/cleanup.yml/badge.svg)](https://github.com/arinbalyan/JobHunter/actions/workflows/cleanup.yml)

</div>

---

## Features

- **Smart Scraping** -- 55+ job boards via [scrappy](https://github.com/arinbalyan/scrappy) engine
- **Intelligent Matching** -- Experience-level aware, role matching, seniority comparison
- **Personalized Emails** -- Context-aware templates for qualified/underqualified/overqualified scenarios
- **Email Tracking** -- Open/click tracking via invisible pixel, bounce/reply detection via IMAP
- **Multi-Provider LLM** -- Dynamic round-robin across 10+ providers with automatic free model discovery
- **Plugin Architecture** -- Drop-in plugins for custom bots and workflows
- **Stats Pipeline** -- Time-series stats collection across all plugins
- **Auto Migrations** -- Embedded SQL migrations via golang-migrate, safe to run repeatedly
- **Memory Optimized** -- Runs under 80MB RAM with Go's GC tuning
- **CI/CD Ready** -- GitHub Actions workflow with secrets management

## Prerequisites

- Go 1.26+
- A [NeonDB](https://console.neon.tech) PostgreSQL database (free tier)
- A Gmail account with [App Password](https://myaccount.google.com/apppasswords)
- An [OpenRouter](https://openrouter.ai/keys) API key (or Groq/Cerebras/Together/etc.)

## Quick Start

```bash
# Clone
git clone https://github.com/arinbalyan/JobHunter.git
cd JobHunter

# Setup
cp .env.example .env
# Edit .env with your keys (see docs/guides/001-Getting-Started.md)

# Run migrations (auto-runs on first start)
go run ./cmd/sender

# Deploy tracking server (separate terminal)
go run ./cmd/tracker
```

## Architecture

```
cmd/
├── sender/      -> Main agent (migrations -> plugins -> stats flush)
├── tracker/     -> Email tracking server (pixel + click redirect)
└── migrate/     -> Manual migration tool

internal/
├── plugin/sdk/  -> Plugin interface and contracts
├── migrations/  -> Embedded SQL migrations
├── email/
│   ├── sender/  -> Gmail SMTP with MIME building
│   ├── tracker/ -> HTTP tracking server
│   └── imap/    -> Bounce and reply scanner
├── llm/router/  -> Multi-provider LLM router with dynamic model discovery
├── scraper/     -> Scrappy CLI adapter
├── job/         -> Job filtering and matching
├── template/    -> HTML email templates
├── stats/       -> Time-series stats collector
├── ratelimit/   -> Token bucket rate limiter
└── config/      -> Environment configuration with auto model fetching

plugins/
├── jobhunter.go -> Core job outreach plugin
└── register.go  -> Plugin registration
```

## LLM Providers (10+)

The router dynamically discovers and load-balances across all configured providers:

| Provider | Models | Access |
|----------|--------|--------|
| OpenRouter | 28+ free models (Gemma 4, DeepSeek V4, Llama 405B, etc.) | API key |
| Groq | Llama 3.3 70B, Llama 3.1 8B | API key |
| Together AI | Llama 3.3 70B Turbo | API key |
| DeepInfra | Llama 3.3 70B | API key |
| Fireworks AI | Llama 3.3 70B | API key |
| Hyperbolic | Llama 3.3 70B | API key |
| SambaNova | Meta-Llama 3.3 70B | API key |
| Cerebras | Wafer-scale fast inference | API key |
| NVIDIA NIM | Nemotron models | API key |
| Z.AI | GLM-4 Plus | API key |

OpenRouter models are discovered dynamically at startup by fetching all `:free` models from their API, sorted by context length.

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting Started](docs/guides/001-Getting-Started.md) | Setup and first run |
| [Plugin System](docs/guides/002-Plugin-System.md) | Create custom plugins |
| [Email Tracking](docs/guides/003-Email-Tracking.md) | Open/click/bounce/reply tracking |
| [LLM Router](docs/guides/004-LLM-Router.md) | Multi-provider AI routing |
| [Database Schema](docs/guides/005-Database-Schema.md) | All tables and migration system |
| [GitHub Actions](docs/guides/006-GitHub-Actions.md) | CI/CD setup and secrets |

## Writing a Plugin

```go
package plugins

import "github.com/arinbalyan/jobhunter/internal/plugin/sdk"

type MyPlugin struct {
    sdk.BasePlugin
}

func NewMyPlugin() *MyPlugin {
    return &MyPlugin{
        BasePlugin: sdk.BasePlugin{
            PluginID:   "mybot",
            PluginName: "My Bot",
            PluginDesc: "Does something awesome",
        },
    }
}

func (p *MyPlugin) Execute(ctx context.Context, env sdk.Env) (*sdk.Result, error) {
    apiKey := env.Getenv("API_KEY")  // reads PLUGIN_MYBOT_API_KEY
    env.Logger().Info("running...")
    // env.DB() for database access
    return sdk.SimpleResult("done"), nil
}
```

Register in `plugins/register.go` and done.

## Database

Tables are auto-created via embedded migrations:

| Table | Purpose |
|-------|---------|
| `jobs` | Scraped job postings |
| `emails` | Sent emails with full tracking |
| `stats` | Time-series events across all plugins |
| `applications` | Pipeline tracking (sent, opened, replied, offer) |
| `blacklist` | Bounced/rejected domains |
| `plugin_state` | Plugin health and run counts |

## Contributing

PRs welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

- Create feature branches from `dev`
- Follow Go conventions (`gofmt`, `go vet`)
- Add tests for new plugins
- Update docs for new features

## License

This project is for use and modification with attribution.
You must credit the original repository and author when using, distributing, or building upon this work.
See the [repository](https://github.com/arinbalyan/JobHunter) for full terms.

## Credits

- [scrappy](https://github.com/arinbalyan/scrappy) -- Fast Go-based job scraper with 55+ boards
- [golang-migrate](https://github.com/golang-migrate/migrate) -- Database migrations
- [pgx](https://github.com/jackc/pgx) -- PostgreSQL driver
