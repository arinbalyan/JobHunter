# 006-GitHub-Actions

## Overview
JobHunter runs fully from GitHub Actions -- no server needed for the main pipeline.
Secrets are stored in repository settings, and workflows run on a schedule.

## Workflows

| Workflow | Schedule (IST) | Purpose | File |
|----------|---------------|---------|------|
| **Scrape Jobs** | 6 AM, 12 PM, 6 PM, 12 AM IST | Build scrappy, scrape 55+ boards, filter jobs into queue | `.github/workflows/scrape.yml` |
| **Send Emails** | 8 AM IST (daily) | Generate LLM emails, send pending queue items | `.github/workflows/send.yml` |
| **Follow-up** | 9 AM IST (daily) | Queue follow-ups for sent+no-reply emails | `.github/workflows/followup.yml` |
| **Cleanup** | Sunday 10 PM IST (weekly) | Delete old skipped jobs, mark stale pending | `.github/workflows/cleanup.yml` |
| **Deploy Tracker** | On push to `main` (tracker/ paths) | Deploy tracking server to Vercel | `.github/workflows/deploy-tracker.yml` |
| **Tests** | On push/PR to `dev`/`main` | Run tests, vet, build | `.github/workflows/tests.yml` |

All workflows except `deploy-tracker` can also be triggered manually via `workflow_dispatch`.

## Setup

### 1. Add Repository Secrets
Go to **Settings -> Secrets and variables -> Actions** and add:

| Secret | Required | Description |
|--------|----------|-------------|
| `DATABASE_URL` | Yes | NeonDB connection string |
| `GMAIL_USER` | Yes | Gmail address |
| `GMAIL_APP_PASS` | Yes | Gmail App Password |
| `OPENROUTER_API_KEY` | Yes | OpenRouter API key |
| `TELEGRAM_BOT_TOKEN` | Recommended | Telegram bot token for run reports |
| `TELEGRAM_CHAT_ID` | Recommended | Telegram chat ID for run reports |
| `TRACKING_SERVER_URL` | Recommended | Deployed tracker URL for email pixels |
| `EMAIL_FROM_NAME` | Recommended | Your name (appears as sender) |
| `CONTACT_NAME` | For send workflow | Your name in email footer |
| `CONTACT_EMAIL` | For send workflow | Your email in footer |
| `CONTACT_PHONE` | For send workflow | Your phone in footer |
| `CONTACT_PORTFOLIO` | For send workflow | Your portfolio URL |
| `CONTACT_GITHUB` | For send workflow | Your GitHub URL |
| `CONTACT_LINKEDIN` | For send workflow | Your LinkedIn URL |
| `CONTACT_CODOLIO` | For send workflow | Your Codolio URL |
| `RESUME_DRIVE_LINK` | For send workflow | Resume Google Drive link |
| `GROQ_API_KEY` | Optional | Additional LLM provider |
| `TOGETHER_API_KEY` | Optional | Additional LLM provider |
| `DEEPINFRA_API_KEY` | Optional | Additional LLM provider |
| `FIREWORKS_API_KEY` | Optional | Additional LLM provider |
| `HYPERBOLIC_API_KEY` | Optional | Additional LLM provider |
| `SAMBANOVA_API_KEY` | Optional | Additional LLM provider |
| `CEREBRAS_API_KEY` | Optional | Additional LLM provider |
| `NVIDIA_API_KEY` | Optional | Additional LLM provider |
| `ZAI_API_KEY` | Optional | Additional LLM provider |

### 2. Repository Variables
Add these to **Settings -> Secrets and variables -> Actions -> Variables**:

| Variable | Default | Description |
|----------|---------|-------------|
| `MAX_EMAILS_PER_RUN` | `10` | Max emails per send run |
| `EMAIL_DELAY_SECONDS` | `30` | Delay between emails |
| `DAILY_EMAIL_LIMIT` | `500` | Gmail's daily cap |

### 3. Verify Secrets Are Not Leaked
- `.env` is in `.gitignore` -- never committed
- Only non-sensitive defaults in `.env.example`
- All actual keys go through GitHub Secrets or local `.env`

## Sync Secrets via CLI

Instead of manually adding secrets to GitHub, use the `syncsecrets` command:

```bash
# Requires: gh CLI installed and authenticated (gh auth login)
go run ./cmd/syncsecrets/
```

This reads your `.env` file and pushes every variable as a GitHub secret.

## Security Best Practices
- Never commit `.env` or actual API keys
- Use Gmail App Passwords (not your real password)
- Rotate API keys periodically
- NeonDB provides connection pooling and SSL by default

## Local Development
```bash
cp .env.example .env
# Fill in your keys

# Doctor checks everything
go run ./cmd/doctor/

# Scrape
go run ./cmd/scrape/

# Send (dry run first)
go run ./cmd/send/ --dry-run
```
