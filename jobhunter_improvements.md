# JobHunter Improvements

Found during development. Items to improve JobHunter itself (not scrappy — see `scrappy_improvements.md` for scrappy-side items).

## Priority: High

### 1. Send doesn't differentiate onsite vs remote

`send` processes all pending emails together. Jobs scraped with `--mode remote` and `--mode onsite` are mixed. Same email template for both.

**Fix**: 
- Migration: `ALTER TABLE jobs ADD COLUMN scrape_mode TEXT`
- `insert_job()` stores `mode` on each row
- `send` gets `--mode remote|onsite` flag to filter which emails to process
- Optionally: separate email prompt templates for onsite vs remote in config.toml

### 2. Scrape stores no per-site job counts

After a scrape, we know total jobs received/inserted but not which sites contributed what. Telegram scrape report shows totals only.

**Fix**: Use scrappy's `ScrapeResult` (item 3 from scrappy backlog — `SiteResult` with per-site stats) to report per-site breakdown in Telegram and dashboard.

### 3. No migration for `jobs.scrape_mode` and `jobs.llm_score` on fresh DB

Migrations exist but a fresh database needs all migrations to run. Currently `sqlx::migrate!()` handles this, but manually running `psql -f migrations/*.sql` misses some columns if tables already exist.

**Fix**: Verify all migrations are idempotent. Add `IF NOT EXISTS` guards where missing.

## Priority: Medium

### 4. Dashboard needs Vercel env vars

Vercel dashboard at jobhunter-tracker.vercel.app needs `DATABASE_URL` set as environment variable. If it expires or gets rotated, the dashboard breaks silently.

**Fix**: Document Vercel env var requirements in deployment docs. Add a `/doctor` endpoint to the Vercel API that checks `DATABASE_URL` and reports status.

### 5. Telegram report missing for send workflow

Scrape gets a Telegram report with stats. Send runs without any notification. If the send workflow fails (quota exhausted, SMTP down), there's no alert.

**Fix**: Add `telegram::send_send_report()` similar to `send_scrape_report()`. Call it after send completion with send/skipped/failed counts.

### 6. config.toml has no validation

Config parsing errors surface as unhelpful TOML parse errors. Unknown sections are silently ignored. Missing required fields (`DATABASE_URL`, LLM API keys) only surface at runtime.

**Fix**: Add a `doctor` command that validates all config sections, checks required env vars, and reports specific fix instructions.

### 7. VERCEL_TOKEN expires

The token used for Vercel deployment expires periodically. There's no automated refresh or alert when it's about to expire.

**Fix**: Document token refresh process. Add a GH Actions weekly check that tests the token and files an issue if expired.

## Priority: Low

### 8. No dry-run mode for `send`

`send` generates and sends in one pipeline. There's no way to preview generated emails before sending.

**Fix**: Add `--dry-run` flag to `send`. Generate emails but mark them as `preview` instead of `generated`, print subject + first 200 chars.

### 9. Onsite mode has no India-specific email templates

Remote and onsite use the same email system prompt. Onsite jobs in India might warrant shorter, more direct emails vs remote roles.

**Fix**: Add `[templates.email_system_onsite]` and `[templates.email_system_remote]` sections. Fall back to `email_system` if mode-specific template missing.

### 10. Release notes include old copilot PRs

The first release (v0.1.1) included PRs from the old Go/copilot era in its changelog. Release notes should filter by time window or only include commits since the last release tag.

**Fix**: In `release.yml`, use `git log` between the last two tags instead of listing all merged PRs.

---

## Cross-references

- **scrappy improvements**: See `scrappy_improvements.md` for scrappy-side items (14/17 done, 3 skipped).
- **scrappy v0.3.9**: Per-site timeout, SiteInfo(), Playwright detection — all available.
- **scrappy v0.3.10**: ScrapeResult with per-site stats (item 3) — not yet released. Would fix item 2 above.
