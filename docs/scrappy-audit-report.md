# Scrappy v0.1.21 Audit Report
Generated: 2026-06-08

## Summary
- **8 total runs analyzed** (Jun 6 19:52 UTC through Jun 8 16:34 UTC, ~2.5 days)
- **7 completed runs** (all success), **1 in-progress**
- **22 boards** producing jobs per run average
- **~405,829 total jobs** per full run (2,840,803 across 7 completed runs)
- **~28,000 WARN/ERROR lines** per run (inflated by per-location-per-term fan-out)
- **12 boards with issues** requiring attention
- **Average runtime**: ~3-4 hours per run (~2,000s for scrape, some runs up to 4.5h)
- **Memory cap**: 100MB, consistently exceeded on longer runs (peaked at 2,017MB on run 27134405205)

---

## Working Boards (return jobs + emails)

Data from run 27134405205 (2026-06-08 11:25 UTC):

| Board | Avg Jobs | Notes |
|-------|----------|-------|
| indeed | 240,000 | Dominant board, US locations work well; non-US locations (Singapore, Dubai, Shanghai, Beijing, Tokyo) return "no jobs found" |
| ycjobs | 37,128 | Y Combinator jobs, stable |
| remoteok | 40,392 | Remote job board, stable |
| workingnomads | 15,912 | Working nomads board, stable |
| remotive | 11,424 | Remote jobs, stable |
| 4dayweek | 10,200 | 4-day work week jobs, stable |
| freelancercom | 9,575 | Freelancer.com, stable |
| mycareersfuture | 9,350 | Singapore career board, stable |
| jobindex | 6,834 | Jobindex (DK), stable |
| internshala | 6,120 | India internship board, stable |
| gunio | 5,712 | Go work board, stable |
| aijobs | 3,492 | AI-focused board, despite rate limiting |
| himalayas | 3,075 | Himalayas board, stable |
| ukvisajobs | 2,448 | UK visa jobs, stable |
| builtin | 2,023 | Built In, despite heavy rate limiting (partial results) |
| linkedin | 407 | LinkedIn, despite heavy rate limiting (partial results) |
| reed | 659 | Reed UK, stable |
| dice | 789 | Dice US, stable for US locations |
| wordpressjobs | 17 | Very low, WordPress-specific niche |
| themuse | 51 | The Muse, stable but low volume |
| cryptocurrencyjobs | 153 | Crypto niche, low volume |
| jobspresso | 68 | Jobspresso, stable but low volume |

---

## Boards With Issues

| Board | Error Type | Details | Impact | Severity |
|-------|-----------|---------|--------|----------|
| **builtin** | Rate limited (HTTP 429) | Every single location-term combination 429s on attempt 1, retry, fail. `partial=510-750` jobs still returned from partial feed. Seen across ALL runs. | Partial data (2k jobs vs potential) | HIGH |
| **linkedin** | Rate limited (HTTP 429) | All locations hit 429 after 2 retries. Some locations (e.g. New York, "LLM Engineer") also show "no parseable jobs". `partial=100-106` per call. | Heavy restriction, but some jobs get through | HIGH |
| **aijobs** | Rate limited (HTTP 429) | Main endpoint `aijobs.ai/remote` consistently 429s after 2 retries. `partial=360-540` jobs returned. | Reduced throughput | MEDIUM |
| **hiringcafe** | Server errors (HTTP 500) | Returns `retryable status` (500). Retries up to 4 times, always fails. `partial=0`. Present in ALL runs. | **Complete failure** | CRITICAL |
| **simplyhired** | Cloudflare blocked | `blocked - cloudflare challenge detected` on page 1. Every single request blocked. Present in all runs. | **Complete failure** | CRITICAL |
| **stepstone** | Timeout | `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`. All 15+ location searches time out. Present in all runs. | **Complete failure** | CRITICAL |
| **greenhouse** | Misconfigured | `greenhouse no seeds: set SCRAPPY_GREENHOUSE_SEEDS env var`. No company slugs configured. Fails instantly for ALL terms/locations. | **Complete failure** (config fix required) | HIGH |
| **dribbble** | Parse failure + HTTP 405 | `dribbble no parseable jobs` for earlier terms, then `dribbble status 405` after ~30s. Dribbble is a design portfolio, not a job board. | **Complete failure** (possibly wrong board) | MEDIUM |
| **dice** | No jobs found | `dice no jobs found` for non-US locations (Dubai, Shanghai, Beijing, Tokyo, Sydney, Paris, Amsterdam). Works for US. | No international coverage | LOW |
| **indeed** | No jobs found | Same pattern: works for US locations, but non-US (Singapore, Dubai, Shanghai, Beijing, Tokyo) show `indeed: no jobs found`. | No international coverage | LOW |
| **jobspresso** | No parseable jobs | `jobspresso no parseable jobs` across all locations and terms. Site renders differently than parser expects. | **Complete failure** | MEDIUM |
| **ats-jobvite** | Timeout | `jobvite fetch: request: Get "...": context deadline exceeded`. Seed config "golang" can't reach Jobvite API. | No ATS-sourced jobs | LOW |

---

## Performance Issues

### Memory Pressure (CRITICAL)
Memory cap is set to **100MB** but is consistently exceeded:

| Run | Peak Memory | Cap | Excess |
|-----|------------|-----|--------|
| 27134405205 (Jun 8 11:25) | **2,017 MB** | 100 MB | +1,917 MB |
| 27117298265 (Jun 8 05:09) | 87-89 MB* | 100 MB | Near limit |
| 27103068811 (Jun 7 19:54) | **187 MB** | 100 MB | +87 MB |
| 27095312419 (Jun 7 14:28) | **320 MB** | 100 MB | +220 MB |
| 27089019428 (Jun 7 09:46) | 54 MB | 100 MB | OK |
| 27083282469 (Jun 7 05:01) | **88 MB** | 100 MB | Near limit |
| 27072237952 (Jun 6 19:52) | 63-64 MB | 100 MB | OK |

\* Memory warnings were emitted at 87%+ consistently starting at 50s into the run.

The `memory_pressure` WARN fires multiple times per run showing `alloc_mb=N cap_mb=100 pct=N gc_cycles=0`. **GC cycles are reported as 0**, suggesting Go GC is not being triggered effectively, allowing unbounded heap growth.

### Runtime
- Average full run: **~2,000-3,000 seconds** (33 min to 4.5 hours)
- The longest runs exceed GitHub Actions 6-hour timeout? Actually they complete within 2-4.5 hours
- Notably, the WARN/ERROR filtering shows the last sample at very different timestamps depending on how boards delay

### Rate Limiting Impact
Heaviest rate limiting seen on:
- `builtin` — all 15+ locations, 2 search terms
- `linkedin` — all 15+ locations, 2 search terms
- Each rate-limited request wastes ~2 retries at ~0.5s each

### Concurrency
- `go=N` starts at ~78 goroutines, tapers to ~5 by end of run
- High goroutine count (75-80) suggests aggressive parallelism for 15+ locations x 10+ terms per board

---

## Error Pattern Analysis

### Rate Limiting (429) — Dominant Error
Occurs on 3 boards consistently. Pattern:
1. `http_roundtrip_status_retry attempt=1 status=429` (first try blocked)
2. `http_roundtrip_failed_rate_limit status=429 attempt=2` (second try also blocked)
3. Board reports `fail_open reason=rate_limited err=...permanent: rate limited (repeated 429)`

UBLO/request pacing is not effective for these boards.

### Server Errors (500/retryable)
- `hiring.cafe` consistently returns HTTP 500. The endpoint may need different parameters or the site actively blocks scrapers.
- After 4 retries: `http_roundtrip_failed err=retryable status`

### Cloudflare Challenges
- `simplyhired.com` actively blocks with Cloudflare JS challenge. No bypass in current HTTP-only client.

### Timeouts
- `stepstone.de` consistently times out. Likely due to slow German server or firewall blocking.

### Misconfiguration
- `greenhouse` missing `SCRAPPY_GREENHOUSE_SEEDS` env var. This is a config issue, not a scrappy bug.

### Parse Failures
- `dribbble` returns empty parse (no jobs) followed by HTTP 405 — site doesn't serve jobs listings the way scrappy expects.
- `jobspresso` returns no parseable jobs across all runs.
- `dice` and `indeed` return "no jobs found" for non-US locations — possibly a geography-based content filtering on the site.

---

## Board Health Matrix (across 7 completed runs)

| Board | Run 1 | Run 2 | Run 3 | Run 4 | Run 5 | Run 6 | Run 7 | Status |
|-------|-------|-------|-------|-------|-------|-------|-------|--------|
| indeed | OK | OK | OK | OK | OK | OK | OK | Stable |
| ycjobs | OK | OK | OK | OK | OK | OK | OK | Stable |
| remoteok | OK | OK | OK | OK | OK | OK | OK | Stable |
| builtin | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | Rate-limited |
| linkedin | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | Rate-limited |
| dice | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | US-only |
| hiringcafe | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | **Broken** |
| simplyhired | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | **Broken** |
| stepstone | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | **Broken** |
| greenhouse | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | **Misconfigured** |
| dribbble | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | **Wrong board** |
| jobspresso | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | FAIL | **Broken** |
| aijobs | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | PARTIAL | Rate-limited |

---

## Recommendations for Scrappy Team

### Immediate (Critical)
1. **Fix memory leak in long-running scrapes**: Memory grows from ~50MB to 2,000MB+ over 30+ minutes with `gc_cycles=0`. Investigate why Go GC is not triggering. Likely cause: accumulating job post structs across all boards/terms without releasing intermediate results. Consider:
   - Periodic `runtime.GC()` calls
   - Streaming results to DB instead of batching
   - Setting `GOGC=50` or lower
   - Using `-gcflags='-d=clobber'` for debugging

2. **Remove or fix hiringcafe board**: HTTP 500 on every request in every run across 2.5 days. Either the board scraper needs updated parameters/headers, or the site has changed its API. Investigate if the site requires a specific `User-Agent` or `Accept` header.

3. **Remove or fix simplyhired board**: Cloudflare JS challenge is non-bypassable with HTTP client. Consider removing the board entirely or switching to a headless browser approach.

4. **Remove or fix stepstone board**: Consistent timeout across all runs. German job board likely rate-limiting or blocking from GitHub Actions IP ranges.

### Medium Priority
5. **Fix greenhouse board**: Add `SCRAPPY_GREENHOUSE_SEEDS` env var to GitHub Actions secrets with known company slugs. Currently instant-fails for hundreds of term-location combinations.

6. **Add request pacing/delay for rate-limited boards**: `builtin` and `linkedin` consistently 429 on first attempt. Adding 1-2s delay between requests per board would reduce retries.

7. **Evaluate dribbble board**: Dribbble is a design portfolio site, not a job board. The scraper may have been written for a Dribbble jobs section that no longer exists. Consider removing.

8. **Evaluate jobspresso board**: Returns no parseable jobs. Site HTML structure may have changed. Update parser or remove.

### Low Priority
9. **Investigate dice/indeed international coverage**: Both boards return "no jobs found" for non-US locations. This may be expected (region-locked content), but should be documented.

10. **Reduce per-term-per-location fan-out**: Currently spawning goroutines for each (board x term x location) combination creates ~22 boards x 12 terms x 15 locations = ~3,960 goroutines at peak. This contributes to memory pressure.

### Monitoring
11. **Add run log sampling/truncation**: 25k+ WARN/ERROR lines per run from per-location-per-term fan-out makes log analysis impractical. Consider aggregating duplicate errors per board.

12. **Track success rate per board over time**: Add board-level success metrics (jobs returned vs expected, error counts) to the run log summary for trend analysis.
