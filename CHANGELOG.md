# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this
project adheres to [Semantic Versioning](https://semver.org/).

---

## [1.0.0] - 2025-02-14

### Added

- Multi-board job scraping via python-jobspy (Indeed, LinkedIn, Glassdoor,
  Naukri)
- Two-phase pipeline: scrape-and-save, then process-and-email with per-row
  status tracking
- LLM-powered email generation via OpenRouter with automatic fallback to
  template-based generation
- Resume PDF attachment on all outgoing emails
- Google Sheets storage backend with three worksheets: Scraped Jobs (22
  columns), Sent Emails (12 columns), Run Stats (15 columns)
- CSV storage backend for local testing
- Three-tier deduplication: exact match, domain cooldown, company cooldown
- Configurable title and email pattern filtering
- Dry run mode for safe testing
- Built-in APScheduler-based scheduling
- Weekend skip logic
- Email summary reports after each run
- GitHub Actions workflows for CI and scheduled scraping
- Full environment-driven configuration via .env
- 200 unit tests with pytest
