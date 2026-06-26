# JobHunter

AI-powered job application pipeline. Scrapes 100+ job boards, generates personalized outreach emails, and sends them — all at $0 in LLM costs.

```bash
# Quick start
curl -fsSL https://github.com/arinbalyan/jobhunter/releases/latest/download/jobhunter-linux.tar.gz | tar xz
./jobhunter doctor          # verify setup
./jobhunter scrape --mode remote   # scrape remote jobs
./jobhunter send --mode remote     # generate + send emails
```

---

## Architecture

```
jobhunter (Rust binary, user-facing)
  │
  ├── calls → scraper (Go bridge, invisible)
  │               │
  │               └── scrappy (Go library, 141 sites)
  │
  ├── LLM router (9 free providers, weighted round-robin)
  ├── SMTP sender (Gmail 587 STARTTLS)
  ├── Tracking server (open/click pixels)
  └── Telegram alerts (per-run reports)
```

Two binaries in one tarball. The Go `scraper` is a thin ~100-line bridge that calls [scrappy](https://github.com/arinbalyan/scrappy) — a Go-native job-board scraper with 141 sites, email extraction, MX verification, and quality scoring.

## Features

| Feature | How |
|---------|-----|
| **100+ job boards** | scrappy handles LinkedIn, Indeed, Greenhouse, and 138 more |
| **ATS discovery** | 28 ATS providers (Ashby, Workable, Greenhouse, SmartRecruiters...) |
| **Email extraction** | scrappy finds + MX-verifies recruiter emails from descriptions and company pages |
| **LLM email generation** | Personalized emails via any of 9 free providers (OpenRouter, Groq, Cerebras...) |
| **Free LLM router** | Weighted round-robin + failover across free-tier models. $0 cost. |
| **Job scoring (1-10)** | LLM classifies each job as a match score |
| **Company research** | Optional LLM enrichment with 3 talking points per company |
| **Reply triage** | Auto-classifies recruiter replies as positive/negative/neutral |
| **Email tracking** | 1x1 pixel opens + click redirect |
| **Concurrent pipeline** | Parallel LLM generation, rate-limited sending |
| **Onsite / Remote modes** | Different search presets in one config file |
| **Telegram alerts** | Per-workflow HTML stats reports |
| **Postgres persistence** | Cloud NeonDB — survives GitHub Actions runner restarts |

## Installation

### From tarball (recommended)

```bash
# Linux x86_64
curl -fsSL https://github.com/arinbalyan/jobhunter/releases/latest/download/jobhunter-linux.tar.gz \
  | tar xz -C /usr/local/bin

# macOS ARM64 (coming soon)
```

### From source

```bash
git clone https://github.com/arinbalyan/jobhunter.git
cd jobhunter
cargo build --release
cp target/release/jobhunter /usr/local/bin/
cd scraper
go build -o ../scraper ./main.go
```

## Configuration

Single file: `.data/config.toml`

```toml
[user]
name = "Arin Balyan"
resume_url = "https://drive.google.com/..."

[search.remote]
terms = ["AI Engineer", "ML Engineer"]
locations = ["remote"]
sites = ["linkedin", "indeed", "greenhouse", "remoteok", "himalayas"]
remote_only = true

[search.onsite]
terms = ["software engineer", "backend developer"]
locations = ["bangalore", "mumbai"]
sites = ["linkedin", "indeed", "internshala", "naukri"]
remote_only = false

[email]
daily_limit = 50
delay_seconds = 15
```

API keys go in `.env` (gitignored). See `.env.example` for all options.

## Commands

| Command | Description |
|---------|-------------|
| `jobhunter scrape --mode <remote\|onsite>` | Scrape jobs → database |
| `jobhunter send --mode <remote\|onsite>` | Generate + send emails |
| `jobhunter doctor` | Diagnose setup issues |
| `jobhunter inbox` | View telemetry dashboard |
| `jobhunter tracker` | Start tracking server |

## Scrappy

This project would not exist without [scrappy](https://github.com/arinbalyan/scrappy) — the Go-native job scraper that powers all job discovery. scrappy is developed independently and consumed as a direct Go library import via the thin `scraper/` subprocess bridge.

**Key scrappy capabilities used by JobHunter:**
- **141 sites** — 49 returning jobs out of the box, 28 ATS providers, 15 with API key support
- **Email extraction** — multi-stage pipeline: HTML mailto: parsing, deobfuscation, regex extraction, company page enrichment, MX DNS verification
- **Quality scoring** — deterministic 0-100 score per job (salary, email verification, freshness, direct-apply, etc.)
- **Per-site rate limiting** — configurable RPS per site, global concurrency semaphore
- **Proxy support** — SOCKS5/HTTP health-checked round-robin
- **Memory-aware** — configurable cap with automatic GC and concurrency scaling

See the scrappy docs at `~/projects/scrappy/documentation/` for:
- [001-Quickstart.md](https://github.com/arinbalyan/scrappy/blob/main/documentation/001-Quickstart.md)
- [005-Status.md](https://github.com/arinbalyan/scrappy/blob/main/documentation/005-Status.md) — current working/broken site status

## LLM Providers (All Free Tier)

JobHunter routes LLM calls across 9 free providers using weighted round-robin with automatic failover:

| Provider | Complex Model | Rate Limit |
|----------|--------------|------------|
| OpenRouter | google/gemma-4-31b-it:free | 20 RPM |
| Groq | openai/gpt-oss-120b | 30 RPM |
| Together | google/gemma-4-9b-it | Varies |
| DeepInfra | Llama-3.3-70B-Instruct-Turbo | Varies |
| Hyperbolic | qwen3-coder-480b-a35b-instruct:free | Varies |
| SambaNova | Meta-Llama-3.3-70B-Instruct | $5 free credits |
| Cerebras | zai-glm-4.7 | Free tier |
| Z.AI | GLM-4-Plus | Free tier |

All models confirmed free as of June 2026. No payment required.

## Deployment

### GitHub Actions (recommended)

4x daily scrape, daily send, weekly cleanup. Secrets set as repository variables.

### Self-hosted

```bash
# Serve the tracking server (for open/click tracking)
jobhunter tracker --port 8080
```

## Project Status

Phase 1 of 5 — see [AGENTS.md](AGENTS.md) for the full roadmap.

```
Current: Scaffold + Config + DB building...
Next:    Scrape Workflow → Send Workflow + LLM → Tracker → Polish & Deploy
```

## License

MIT
