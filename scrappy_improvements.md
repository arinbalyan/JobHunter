# scrappy Improvements

Found while integrating scrappy v0.3.7 into JobHunter. When you're free, pick items from here.

**Final tally: 14/17 done ✅ · 1 blocked forever ❌ · 2 skipped ⏭️**

> **JobHunter improvements**: See `jobhunter_improvements.md` for JobHunter-side items (send mode, per-site stats, Vercel, etc.).
> This file is for **scrappy** changes only.

## To add for JobHunter (not scrappy)

### `EmailsOnly` changes output format (return type differs)

When `EmailsOnly: true`, scrappy returns `Emails` as `[]string` (simple email strings). When `false`, it returns `[]Email` objects with `addr`, `verified`, `source`, `role`. This means consumers that expect objects break when toggling the flag.

**Fix**: Always return `[]Email` regardless of `EmailsOnly`. The flag should only control whether jobs without emails are included, not change the data type of the emails field.

### Determine remote/onsite per-job during scrape

scrappy already has `IsRemote bool` on `JobPost` (line 395 of `internal/model/types.go`). But it's not always reliably set — some scrapers leave it as `false` even for remote jobs, others don't populate it at all.

scrappy **can** determine this more accurately:
- **Site-level**: remote-only boards (`remoteok`, `weworkremotely`, `himalayas`, `ycjobs`) → all jobs are `IsRemote=true`
- **Location-level**: if job posting says "remote" in any location field → `IsRemote=true`
- **Input-level**: if `ScraperInput.RemoteOnly` is true → all returned jobs are remote (the consumer requested only remote jobs)
- **Explicit**: some job postings have a "remote" or "work-from-home" flag in their API response

**Fix**: Add a post-processing step after each site's scrape that normalizes `IsRemote` using the signals above. This is already partially done in some scrapers but not consistently.

**JobHunter benefit**: If `IsRemote` is reliable on every `JobPost`, send can filter by it without needing a separate `scrape_mode` column in the DB. The job itself carries the signal.

### Send doesn't differentiate onsite vs remote (JobHunter fix, not scrappy)

`send` processes all pending emails together regardless of scrape mode. Onsite jobs (Bangalore) and remote jobs get the same email template. The `{location}` placeholder helps the LLM adapt, but the system prompt is identical.

This is a **JobHunter issue**, not scrappy. scrappy's `JobPost.IsRemote` is per-job from the posting data, but the `--mode remote|onsite` flag is a search-level concept that scrappy doesn't track.

**JobHunter fix**:
1. Migration: `ALTER TABLE jobs ADD COLUMN scrape_mode TEXT`
2. `insert_job()` stores the mode on each row
3. `send` gets `--mode` filter, only processes matching emails
4. Separate prompt sets in config.toml for onsite vs remote would allow different email tones/lengths

## Completed

| # | Item | Status |
|---|------|--------|
| 1 | ResultsWanted=0 unlimited | ✅ Already fixed before doc |
| 2 | `WithConfig()` option | ✅ v0.3.8 |
| 3 | Per-site result metadata (`ScrapeResult`) | ✅ |
| 4 | LinkedIn rate limiting (5 req/s token bucket) | ✅ |
| 5 | Slug staleness detection | ✅ |
| 6 | Streaming API (`ScrapeJobsStream`) | ✅ |
| 7 | Richer `SiteInfo()` | ✅ v0.3.9 |
| 9 | Dynamic concurrency (heap pressure scaling) | ✅ |
| 10 | `SiteSkipLocation` | ✅ v0.3.8 |
| 11 | Fuzzy dedup (title+company normalization) | ✅ |
| 12 | Error sentinels (5 error kinds + `ErrorKind()`) | ✅ |
| 13 | `SiteTimeout` per-site | ✅ v0.3.9 |
| 14 | Playwright detection | ✅ v0.3.9 |
| 15 | Config reload (`ReloadConfig()`) | ✅ |

## Skipped

| # | Item | Why |
|---|------|-----|
| 8 | uTLS reintroduction | ❌ Blocked forever — corrupts Go HTTP TLS state machine |
| 16 | Per-site proxy | ⏭️ Too much transport layer refactoring for unclear benefit |
| 17 | ATS rate limiting | ⏭️ Already handled by existing `SiteRPS` mechanism |

## Things scrappy does GREAT (don't touch)

| Feature | Why it's excellent |
|---------|-------------------|
| **Email extraction pipeline** | Multi-stage: HTML mailto:, regex deobfuscation, company page enrichment, MX DNS, optional SMTP |
| **Quality scoring** | Deterministic 0-100, no LLM needed, 8 weighted factors |
| **Per-site concurrency** | Goroutine fan-out with token-bucket rate limiting per site |
| **Fail-open behavior** | One broken site doesn't kill the whole scrape |
| **Site telemetry** | Per-site success/failure/captcha/rate-limit tracking with elapsed time |
| **ATS slug embedding** | 2,291 slugs across 28 providers in a single embedded file |
| **MX verification** | Concurrent DNS lookups with configurable parallelism |
| **LinkedIn guest API** | v0.3.7 works without Playwright |
| **v0.3.7 security fixes** | SSRF fix + credential leak fix |
