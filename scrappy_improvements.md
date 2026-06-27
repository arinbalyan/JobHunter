# scrappy Improvements

Found while integrating scrappy v0.3.7 into JobHunter. When you're free, pick items from here.

**Final tally: 14/17 done ✅ · 1 blocked forever ❌ · 2 skipped ⏭️**

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
