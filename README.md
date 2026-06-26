# JobHunter

AI-powered job application pipeline. Scrapes 100+ job boards, scores jobs, generates personalized outreach emails, and sends them — all at **$0 in LLM costs** via free-tier providers.

```bash
# Quick start
curl -fsSL https://github.com/arinbalyan/jobhunter/releases/latest/download/jobhunter-linux.tar.gz | tar xz
cp config.example.toml .data/config.toml  # edit with your details
./jobhunter doctor                         # verify setup
./jobhunter scrape --mode remote           # scrape remote jobs
./jobhunter score                          # score jobs 1-10
./jobhunter send                           # generate + send emails
```

---

## Architecture

```
jobhunter (Rust binary)
  │
  ├── spawns → scraper (Go bridge, ~30 lines)
  │               │
  │               └── scrappy (Go library: 141 sites, email extraction, MX verify)
  │
  ├── LLM router (9 free providers, weighted random + failover)
  ├── SMTP sender (Gmail 587 STARTTLS, rate-limited, quota tracked)
  ├── Tracking server (open/click pixels)
  └── Telegram alerts (per-run reports)
```

Two binaries in one tarball. The Go `scraper` is a thin bridge to [scrappy](https://github.com/arinbalyan/scrappy).

## Commands

| Command | What it does |
|---------|-------------|
| `jobhunter scrape --mode remote\|onsite` | Scrape jobs → filter → dedup → queue |
| `jobhunter score` | Score unscored jobs 1-10 via LLM |
| `jobhunter research` | Generate 3 talking points per company via LLM |
| `jobhunter send` | Generate emails + send via SMTP |
| `jobhunter serve` | HTTP tracking server (/track, /click, /health) |
| `jobhunter doctor` | Diagnose config, DB, API keys, binary |

## Pipeline

```
Scrape → Score → Research → Send
  │         │        │         │
  │         │        │         └── SMTP (rate-limited, 1/15s)
  │         │        │             └── Tracking pixel + quota tracking
  │         │        └── 3 talking points per company (LLM, simple model)
  │         └── 1-10 match score per job (LLM, simple model)
  └── 141 boards → title filter → email filter → SQL dedup → email_queue
```

## Configuration

Single file `.data/config.toml`. API keys via `$VAR` references from `.env` or GitHub Secrets.

```toml
[user]
name = "Your Name"
resume_url = "https://drive.google.com/..."

[search.remote]
terms = ["AI Engineer", "ML Engineer"]
locations = ["remote"]
sites = ["linkedin", "indeed", "remoteok", "ycjobs"]
remote_only = true

[search.onsite]
terms = ["software engineer", "backend developer"]
locations = ["bangalore", "mumbai"]
sites = ["linkedin", "indeed", "internshala"]
remote_only = false
```

Full reference at [docs/configuration.md](docs/configuration.md).

## LLM Providers (All Free Tier)

| Provider | Complex Model | Weight |
|----------|--------------|--------|
| OpenRouter | google/gemma-4-31b-it:free | 10 |
| Groq | openai/gpt-oss-120b | 5 |
| Together | google/gemma-4-9b-it | 4 |
| DeepInfra | Llama-3.3-70B-Instruct-Turbo | 4 |
| Hyperbolic | qwen3-coder-480b-a35b-instruct:free | 3 |
| SambaNova | Meta-Llama-3.3-70B-Instruct | 3 |
| Cerebras | zai-glm-4.7 | 2 |
| Z.AI | GLM-4-Plus | 1 |

Weighted random selection, failover up to 3 providers, auto-cooldown on failures.

## LLM Features

| Feature | Model | Cost |
|---------|-------|------|
| Email generation | Complex | One call per email |
| Job scoring (1-10) | Simple | One short call per job |
| Company research (3 points) | Simple | One call per company |
| Reply triage (planned) | Simple | One call per reply |

## Deployment

### GitHub Actions (automated)

Two workflows: **scrape** (4x daily) + **send** (daily). Secrets set as repository variables.

### Self-hosted tracking server

```bash
./jobhunter serve
# → /track?e=<uuid>  (1x1 GIF, logs open)
# → /click?e=<uuid>  (302 redirect, logs click)
# → /health
```

## License

MIT
