# JobHunter Documentation

## Guides

| # | Guide | Description |
|---|-------|-------------|
| 001 | [Getting Started](guides/001-Getting-Started.md) | Setup, configuration, first run |
| 002 | [Plugin System](guides/002-Plugin-System.md) | Writing and registering plugins |
| 003 | [Email Tracking](guides/003-Email-Tracking.md) | Pixel tracking, click redirects, IMAP bounce/reply detection |
| 004 | [LLM Router](guides/004-LLM-Router.md) | Multi-provider AI routing, dynamic model discovery, token management |
| 005 | [Database Schema](guides/005-Database-Schema.md) | All tables, migration system, indexes |
| 006 | [GitHub Actions](guides/006-GitHub-Actions.md) | CI/CD setup, secrets, automation |
| 007 | [Commands Reference](guides/007-Commands-Reference.md) | All 11 CLI commands: scrape, send, followup, cleanup, tracker, inbox, doctor, migrate, botid, syncsecrets |
| 008 | [Docker Deployment](guides/008-Docker-Deployment.md) | Docker Compose for local dev, building images, tracker deployment |
| 009 | [Telemetry & Monitoring](guides/009-Telemetry-and-Monitoring.md) | Tracking server, inbox telemetry dashboard, IMAP scanner |
| 010 | [Troubleshooting](guides/010-Troubleshooting.md) | Common issues: scrappy, .env, DB, SMTP, OpenRouter, Docker, CI |

## Reference

- [Plugin SDK](../internal/plugin/sdk/sdk.go) -- Full plugin interface documentation
- [Configuration](../internal/config/config.go) -- All env vars and their defaults
- [Migrations](../internal/migrations/) -- SQL migration files (000001-000005)

## Quick Links

- [README](../README.md) -- Project overview and quick start
- [AGENTS.md](../AGENTS.md) -- AI agent configuration and architecture
