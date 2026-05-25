# 006-GitHub-Actions

## Overview
JobHunter can run fully from GitHub Actions — no server needed.
Secrets are stored in repository settings, and the agent runs on a schedule.

## Setup

### 1. Add Repository Secrets
Go to **Settings → Secrets and variables → Actions** and add:

| Secret | Description |
|--------|-------------|
| `DATABASE_URL` | NeonDB connection string |
| `GMAIL_USER` | Gmail address |
| `GMAIL_APP_PASS` | Gmail App Password |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `GROQ_API_KEY` | (Optional) Groq key |
| `CEREBRAS_API_KEY` | (Optional) Cerebras key |

### 2. Workflow File
The workflow at `.github/workflows/send-emails.yml` runs daily:

```yaml
name: Daily Email Sender
on:
  schedule:
    - cron: '0 9 * * *'   # Daily at 9 AM UTC
  workflow_dispatch:        # Manual trigger

jobs:
  run:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
      - name: Run agent
        run: go run ./cmd/sender
        env:
          DATABASE_URL: ${{ secrets.DATABASE_URL }}
          GMAIL_USER: ${{ secrets.GMAIL_USER }}
          GMAIL_APP_PASS: ${{ secrets.GMAIL_APP_PASS }}
          OPENROUTER_API_KEY: ${{ secrets.OPENROUTER_API_KEY }}
```

### 3. Verify Secrets Are Not Leaked
- `.env` is in `.gitignore` — never committed
- Only non-sensitive defaults in `.env.example`
- All actual keys go through GitHub Secrets or local `.env`

## Local Development
```bash
cp .env.example .env
# Fill in your keys
go run ./cmd/sender
```

## Security Best Practices
- Never commit `.env` or actual API keys
- Use Gmail App Passwords (not your real password)
- Rotate API keys periodically
- NeonDB provides connection pooling and SSL by default
