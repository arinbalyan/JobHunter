# JobHunter

AI-powered job application pipeline. Scrapes 100+ job boards, scores jobs, researches companies, generates personalized outreach emails, and sends them — all at **$0 in LLM costs** via free-tier providers.

```bash
# Quick start
git clone https://github.com/arinbalyan/jobhunter.git
cd jobhunter
cargo build --release
cd scraper && go build -o scraper . && cd ..
cp config.example.toml .data/config.toml   # edit with your details
./target/release/jobhunter doctor           # verify setup
./target/release/jobhunter scrape --mode remote   # scrape remote jobs
./target/release/jobhunter score                  # score jobs 1-10
./target/release/jobhunter send                   # generate + send emails
```

---

## Architecture

```
jobhunter (Rust binary, ~700 lines)
  │
  ├── spawns → scraper (Go bridge, ~30 lines)
  │               │
  │               └── scrappy v0.3.9 (Go library: 141 sites, email extraction, MX verify)
  │
  ├── LLM router (9 free providers, weighted random + failover)
  ├── SMTP sender (Gmail 587 STARTTLS, rate-limited, quota tracked)
  ├── Tracking server (open/click pixels)
  ├── Pipeline dashboard (Vercel)
  └── Telegram alerts (per-run reports)
```

Two binaries in one tarball. The Go `scraper` is a thin bridge to [scrappy](https://github.com/arinbalyan/scrappy).

## Commands

| Command | What it does |
|---------|-------------|
| `jobhunter scrape --mode remote\|onsite` | Scrape 141 boards → filter → dedup → queue |
| `jobhunter score` | Score unscored jobs 1-10 via LLM |
| `jobhunter research` | Generate 3 talking points per company via LLM |
| `jobhunter send` | Generate emails + send via SMTP |
| `jobhunter triage "<reply>"` | Classify recruiter reply (positive/negative/neutral) |
| `jobhunter import --from <scrappy_config>` | Import scrappy per-site config into JobHunter config |
| `jobhunter serve` | HTTP tracking server + pipeline dashboard |
| `jobhunter doctor` | Diagnose config, DB, API keys, binary |

## Pipeline

```
Scrape → Score → Research → Send → Track
  │        │        │         │        │
  │        │        │         │        └── opens (1x1 pixel)
  │        │        │         │        └── clicks (/click?e=&url=)
  │        │        │         └── Gmail SMTP (rate-limited 1/15s)
  │        │        │             └── signature with GitHub/Portfolio/Resume
  │        │        └── 3 talking points per company (LLM, simple model)
  │        └── 1-10 match score per job (LLM, simple model)
  └── 141 boards → title filter → email filter → SQL dedup → email_queue
      scrappy guest API (no Playwright needed)
```

## Configuration

Single file `.data/config.toml`. API keys via `$VAR` references from `.env` or GitHub Secrets.

Key sections:

| Section | Purpose |
|---------|---------|
| `[user]` | Name, role, experience, GitHub, portfolio, resume URL, **LLM context** |
| `[search.remote]` / `[search.onsite]` | Search terms, locations, sites per mode |
| `[scrape]` | Runtime, results limit, **reject_titles**, **blocked_email_*** (all configurable) |
| `scrappy_config` | Path to scrappy's `config.toml` — auto-loads **118 per-site search overrides** |
| `[sites.*]` | Per-site search terms/location overrides (optional, auto-imported) |
| `[[llm.providers]]` | 8 free providers with weights, models, API key env vars |
| `[templates.*]` | All LLM prompts — edit without recompiling |

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
| Email generation | Complex (e.g. Gemma 4 31B) | One call per email |
| Job scoring (1-10) | Simple | One short call per job |
| Company research (3 points) | Simple | One call per company |
| Reply triage | Simple | One call per reply |

## Deployment

### GitHub Actions (automated)

Three workflows:
- **scrape** — 4x daily, builds Rust + Go bridge, scrapes all 78 sites
- **send** — daily, generates + sends emails for queued jobs
- **tests** — on every push, runs `cargo test` + Go vet

### Vercel Dashboard

Live at **https://jobhunter-tracker.vercel.app** — pipeline funnel, per-URL click breakdown, run history, failures. Node.js serverless functions in `api/`.

### Self-hosted tracking server

```bash
./jobhunter serve
# → GET /         pipeline dashboard (HTML)
# → GET /track?e= 1x1 GIF, logs open
# → GET /click?e=&url= 302 redirect, logs click
# → GET /health
```

## License

MIT
