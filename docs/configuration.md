# Configuration

Single file: `.data/config.toml` (searched first) or `config.toml`.

API keys use `$VAR` references resolved from environment at load time. Set them in `.env` (gitignored) or GitHub Secrets.

## Sections

### `[user]`

```toml
[user]
name = "Your Name"
current_role = "AI/ML Engineer"
years_experience = 1
resume_path = ".data/YourName.pdf"    # optional, for email attachments
```

### `[scrape]`

```toml
[scrape]
max_runtime_minutes = 490    # context timeout for the Go scraper bridge
results_wanted = 0           # 0 = unlimited, otherwise caps total results
```

### `[search.remote]` / `[search.onsite]`

Two presets, selected via `--mode remote|onsite`:

```toml
[search.remote]
terms = ['"AI Engineer"', '"ML Engineer"']
locations = ["remote"]
sites = ["linkedin", "indeed", "remoteok", "ycjobs"]
remote_only = true

[search.onsite]
terms = ['"Software Engineer"', '"Backend Developer"']
locations = ["bangalore", "mumbai", "pune"]
sites = ["linkedin", "indeed", "internshala"]
remote_only = false
```

| Field | Description |
|-------|-------------|
| `terms` | Search queries. Each element is sent as-is to each site's search. |
| `locations` | Locations to search (site-dependent, some ignore it) |
| `sites` | Scraper site names (see scrappy docs for full list of 141) |
| `remote_only` | If true, scrappy filters to remote jobs |

### `[telegram]`

```toml
[telegram]
chat_id = "123456789"    # Telegram chat ID for scrape reports
```

Bot token is read from `TELEGRAM_BOT_TOKEN` env var.

### `[llm]`

```toml
[llm]
max_tokens_per_run = 100000
max_tokens_per_request = 2048
temperature = 0.7

[[llm.providers]]
name = "OpenRouter"
api_key_env = "OPENROUTER_API_KEY"
base_url = "https://openrouter.ai/api/v1"
model_complex = "google/gemma-4-31b-it:free"
model_simple = "openrouter/free"
weight = 10
```

Each provider entry:

| Field | Description |
|-------|-------------|
| `name` | Display name (for logs/doctor) |
| `api_key_env` | Env var name containing the API key |
| `base_url` | OpenAI-compatible base URL |
| `model_complex` | Model used for email generation |
| `model_simple` | Model used for lightweight tasks (future: scoring, triage) |
| `weight` | Selection weight (higher = more likely to be picked) |

### `[templates]`

LLM prompts. System prompt defines behavior, user prompt is filled per-job:

```toml
[templates.email_system]
content = """\
You are a professional job applicant writing a cold outreach email...
"""

[templates.email_user]
content = """\
## Applicant Context
{context}

## Target Position
- Title: {title}
- Company: {company}
- Description: {description}
...
```

Placeholders in `email_user`: `{context}`, `{title}`, `{company}`, `{description}`, `{location}`, `{seniority}`, `{job_type}`, `{salary}`, `{skills}`, `{industry}`, `{experience_match}`.

## Env vars

| Variable | Where | Required |
|----------|-------|----------|
| `DATABASE_URL` | .env / GitHub Secret | ✅ Yes |
| `TELEGRAM_BOT_TOKEN` | .env / GitHub Secret | For scrape reports |
| `OPENROUTER_API_KEY` | .env / GitHub Secret | For LLM |
| `GROQ_API_KEY` | .env / GitHub Secret | For LLM |
| `TOGETHER_API_KEY` | .env / GitHub Secret | For LLM |
| `DEEPINFRA_API_KEY` | .env / GitHub Secret | For LLM |
| `HYPERBOLIC_API_KEY` | .env / GitHub Secret | For LLM |
| `SAMBANOVA_API_KEY` | .env / GitHub Secret | For LLM |
| `CEREBRAS_API_KEY` | .env / GitHub Secret | For LLM |
| `ZAI_API_KEY` | .env / GitHub Secret | For LLM |
| `SCRAPPY_INDEED_API_KEY` | .env / GitHub Secret | For Indeed scraping |
| `SCRAPPY_DICE_API_KEY` | .env / GitHub Secret | For Dice scraping |

## CLI

```
USAGE:
    jobhunter <SUBCOMMAND>

COMMANDS:
    scrape    Scrape job boards, filter, and queue emails
              --mode <remote|onsite>     (default: remote)
    send      Generate emails for queued jobs via LLM
              --max <N>                  (default: 10)
    doctor    Run diagnostics
```
