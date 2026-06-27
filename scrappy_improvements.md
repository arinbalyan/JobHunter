# scrappy Improvements

Found while integrating scrappy v0.3.7 into JobHunter. When you're free, pick items from here.

## Priority: High

### 1. ResultsWanted=0 not consistently "unlimited" across scrapers

**Engine level**: `ResultsWanted <= 0` correctly means no limit — no trimming, no capping.
**Per-site**: Varies:
- **Indeed**: Explicitly handles 0 → `math.MaxInt32` (paginate until exhausted) ✅
- **YCJobs**: Guards with `> 0`, so 0 means no cap ✅
- **iCIMS, CareerBuilder, Freshteam, Rippling**: `wanted := input.ResultsWanted` — when 0, limits to 0 results ❌

**Fix**: Standardize: each site scraper should treat `ResultsWanted <= 0` as "get all available" (like Indeed does).

### 2. ✅ `NewEngine(WithConfig())` — Done v0.3.8

Library consumers can now pass per-site search terms from scrappy's `config.toml`:
```go
engine, _ := scrappy.NewEngine(scrappy.WithConfig("config.toml"))
```
`WithConfig` loads the `[sites]` section and sets per-site search terms / location / country on the Engine.

### 3. No per-site result metadata returned

`ScrapeJobs()` returns `[]JobPost` with no information about which sites succeeded/failed, how many jobs each found, or why some failed. Consumers have to parse logs to understand scrape health.

**Fix**: Return a result struct:
```go
type ScrapeResult struct {
    Jobs  []JobPost
    Sites []SiteResult
}
type SiteResult struct {
    Name   string
    Jobs   int
    Error  string // empty if success
}
```

### 4. LinkedIn guest API rate limits hit fast

LinkedIn guest API returns 429 after ~100 requests (10 terms × 10 pages). The current config hits 24 terms × 1 location → 240 requests, which means ~140 get rate-limited.

**Fix**: Add per-term rate limiting specifically for LinkedIn, or reduce the default page count when using guest API. Consider a rotating session ID or exponential backoff between pages.

## Priority: Medium

### 5. Company slug staleness detection

2,291 ATS slugs across 28 providers. Many go stale as companies switch ATS providers. No automatic way to detect dead slugs.

**Fix**: In `ProcessSeeds`, track which slugs return 0 jobs for N consecutive runs. Flag or deprioritize them. Export the staleness data so consumers can update their slug lists.

### 6. No streaming/progress API for library consumers

`ScrapeJobs()` blocks until all sites finish. For 78+ sites, this means the consumer waits minutes before seeing any results. Memory also grows with all results buffered.

**Fix**: Add a streaming variant:
```go
engine.ScrapeJobsStream(ctx, input, func(job JobPost) {
    // called per-job as they arrive
})
```

### 7. ✅ `SiteInfo()` — Done v0.3.9

Returns method + needs_api_key per site.
```go
siteInfo, _ := engine.SiteInfo()
for _, s := range siteInfo {
    fmt.Printf("%s: method=%s needs_key=%v\n", s.Name, s.Method, s.NeedsAPIKey)
}
```

### 8. uTLS/tls fingerprinting was reverted but useful

The uTLS revert (`76451d1`) fixed TLS state machine issues, but losing fingerprinting means some anti-bot sites may now block requests.

**Fix**: Re-introduce uTLS with a per-request transport rather than a global one, so it doesn't interfere with the HTTP transport state machine.

### 9. Memory cap concurrency scaling is fixed tiers

`globalConcurrency()` returns hardcoded tiers (3/5/8/12 goroutines). Not adaptive to actual heap growth.

**Fix**: Dynamic concurrency based on real-time heap pressure (e.g., scale down when GC cycles are frequent).

## Priority: Low

### 10. ✅ `SiteSkipLocation` — Done v0.3.8

The engine now supports skipping location iteration per site:
```go
SiteSkipLocation: map[Site]bool{"remoteok": true, "himalayas": true}
```
Remote-only boards no longer waste time on location combos.

### 11. Dedup within a run is URL-only

Same job posted on multiple boards (same title+company, different URL) won't be deduped.

**Fix**: Optional fuzzy dedup by normalized title + company name. Low priority — URL dedup catches most duplicates.

### 12. Better error type distinction

All errors surface as generic strings. Consumers can't distinguish "site down" vs "no jobs found" vs "rate limited" vs "auth failure" without parsing log text.

**Fix**: Define typed error sentinels or error kinds:
```go
var ErrRateLimited = errors.New("rate limited")
var ErrNoJobs = errors.New("no jobs found")
```
Include error kind in `SiteResult` (item 3).

### 13. ✅ `SiteTimeout` — Done v0.3.9

Per-site timeout override on ScraperInput:
```go
SiteTimeout: map[Site]time.Duration{"linkedin": 120 * time.Second, "remoteok": 10 * time.Second}
```
Default to global timeout when not specified.

### 14. ✅ Playwright detection — Done v0.3.9

`playwrightCheck()` runs once at startup, caches result. Clear error when missing:
```
site: requires Playwright but Node.js or playwright module is not installed
(run: npx playwright install chromium)
```

### 15. Config reload without restart

Changing search terms in `config.toml` requires restarting the process. For long-running consumers (like JobHunter's tracking server), this is disruptive.

**Fix**: Add a `ReloadConfig()` method on Engine that re-reads the config file and updates per-site search terms / locations without restarting.

## Priority: Low

### 16. Per-site proxy support

Some sites (LinkedIn, Indeed) benefit from proxies to avoid rate limits. Others (RemoteOK, YCJobs) never block. Currently proxy is all-or-nothing via env vars.

**Fix**: Allow per-site proxy assignment in config.toml:
```toml
[sites.linkedin]
proxy = "socks5://user:pass@proxy:1080"
```

### 17. ATS provider rate limiting

Some ATS providers (Greenhouse, Ashby) have aggressive rate limits when scraping 100+ companies. The current per-site token bucket doesn't account for the extra load from ATS slug processing.

**Fix**: Separate ATS rate limiting from regular site rate limiting. Allow setting a separate RPS for ATS seed processing.

---

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
| **LinkedIn guest API** | v0.3.7 works without Playwright (HUGE improvement for deployment) |
| **v0.3.7 security fixes** | SSRF fix + credential leak fix are critical |
