# Configuration

Single file: `.data/config.toml` (searched first) or `config.toml`.

API keys use `$VAR` references resolved from environment at load time. Set them in `.env` (gitignored) or GitHub Secrets.

## Top-level fields

```toml
# Optional: path to scrappy's config.toml for per-site search terms
scrappy_config = "~/projects/scrappy/config.toml"
```

When `scrappy_config` is set, the Rust code loads scrappy's `config.toml` and passes its `[sites]` section to the bridge as `site_search`/`site_location` — so each site gets its optimized search terms instead of global defaults.

## `[user]`

```toml
[user]
name = "Arin Balyan"
current_role = "AI/ML Engineer"
years_experience = 1
github = "https://github.com/arinbalyan"
portfolio = "https://arinbalyan.vercel.app"
resume_url = "https://drive.google.com/..."   # Appended to email signature
```

## `[scrape]`

```toml
[scrape]
max_runtime_minutes = 490     # Bridge context timeout
results_wanted = 0            # 0 = unlimited
reject_titles = ["senior", "manager", "intern", ...]   # Title patterns to skip
blocked_email_prefixes = ["no-reply", "noreply", ...]  # Email prefixes to filter
blocked_email_contains = ["accessibility", ...]         # Email substrings to filter
blocked_tlds = [".tk", ".ml", ".xyz", ...]             # TLDs to filter
```

All filter patterns (`reject_titles`, `blocked_email_*`) are read from config at runtime — no recompilation needed. Full lists from the Go dev branch are included.

## `[search.remote]` / `[search.onsite]`

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

## `[sites]` (per-site overrides)

Optional. Overrides global search terms/locations for specific sites. Usually imported from scrappy's config:

```toml
[sites.remoteok]
search_terms = ["software engineer", "full stack", "backend"]
location = "Remote"

[sites.linkedin]
search_terms = ['"Software Engineer" OR "Full Stack Developer"']
```

118 sites imported by default via the `import` command.

## `[telegram]`

```toml
[telegram]
chat_id = "123456789"
```

Bot token from `TELEGRAM_BOT_TOKEN`.

## `[llm]`

```toml
[[llm.providers]]
name = "OpenRouter"
api_key_env = "OPENROUTER_API_KEY"
base_url = "https://openrouter.ai/api/v1"
model_complex = "google/gemma-4-31b-it:free"
model_simple = "openrouter/free"
weight = 10
```

## `[templates]`

LLM prompts. All configurable:

```toml
[templates.email_system]
content = """..."""

[templates.email_user]
content = """..."""
```

## Env vars

| Variable | For |
|----------|-----|
| `DATABASE_URL` | Postgres (NeonDB) |
| `TELEGRAM_BOT_TOKEN` | Telegram reports |
| `OPENROUTER_API_KEY` → `ZAI_API_KEY` | LLM providers |
| `SCRAPPY_INDEED_API_KEY` | Indeed scraping |
| `SCRAPPY_DICE_API_KEY` | Dice scraping |

## CLI

```
jobhunter scrape --mode remote|onsite    Scrape → filter → dedup → queue
jobhunter score                          Score unscored jobs 1-10
jobhunter research                       Research 3 talking points per company
jobhunter send                           Generate + send emails
jobhunter triage "<reply>"               Classify recruiter reply
jobhunter import --from <scrappy_config> Import scrappy per-site config
jobhunter serve                          Tracking server + dashboard
jobhunter doctor                         Diagnose everything
```
