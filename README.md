# JobSpy V2

Automated job scraping and cold email outreach system.

## Quick Start

```bash
# Install dependencies
uv sync

# Configure
cp .env.example .env
# Edit .env with your values (see .env.example for detailed setup instructions)

# Run onsite workflow
uv run python -m jobspy_v2 onsite

# Run remote workflow
uv run python -m jobspy_v2 remote

# Dry run (no emails sent)
uv run python -m jobspy_v2 onsite --dry-run
```

## Setup

See `.env.example` for comprehensive step-by-step setup instructions for:
- Gmail App Password
- OpenRouter API key
- Google Sheets integration
- Job search configuration

## Architecture

```
src/jobspy_v2/
├── config/        # Pydantic settings, defaults
├── core/          # Scraper, email gen, sender, dedup, reporter
├── storage/       # Google Sheets (primary) + CSV (fallback)
├── scheduler/     # APScheduler for PaaS deployment
├── workflows/     # Base + onsite/remote pipelines
└── utils/         # Email/text utilities
```

## License

MIT
