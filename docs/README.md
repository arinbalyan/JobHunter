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

## Reference

- [Plugin SDK](../internal/plugin/sdk/sdk.go) -- Full plugin interface documentation
- [Configuration](../internal/config/config.go) -- All env vars and their defaults
- [Migrations](../internal/migrations/) -- SQL migration files
